package apk

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	pb "github.com/jeriveromartinez/sofascore-scrapper/internal/gen/api"
	"gorm.io/gorm"
)

type UploadPublication struct {
	ID        uint           `gorm:"primaryKey"`
	UploadID  string         `gorm:"uniqueIndex;size:36;not null;column:upload_id"`
	TempPath  string         `gorm:"size:1024;not null;column:temp_path"`
	FinalPath string         `gorm:"size:1024;not null;default:'';column:final_path"`
	Status    string         `gorm:"size:20;not null;default:'assembling';column:status"`
	UserID    uint           `gorm:"not null;default:0;column:user_id"`
	CreatedAt time.Time      `gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"not null;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (UploadPublication) TableName() string {
	return "apk_upload_publications"
}

type UploadService struct {
	store      *UploadStateStore
	chunkStore *ChunkStore
	repo       *Repository
	db         *gorm.DB
}

func NewUploadService(store *UploadStateStore, chunkStore *ChunkStore, repo *Repository, db *gorm.DB) *UploadService {
	return &UploadService{
		store:      store,
		chunkStore: chunkStore,
		repo:       repo,
		db:         db,
	}
}

func (s *UploadService) Begin(ctx context.Context, userID uint, req *pb.UploadBeginRequest) (*pb.UploadBeginResponse, error) {
	if req.TotalChunks <= 0 || req.TotalChunks > MaxTotalChunks {
		return nil, fmt.Errorf("total_chunks must be between 1 and %d", MaxTotalChunks)
	}
	if req.FileSize <= 0 || req.FileSize > MaxAggregate {
		return nil, fmt.Errorf("file_size must be between 1 and %d", MaxAggregate)
	}
	if req.FileName == "" {
		return nil, fmt.Errorf("file_name is required")
	}

	session, err := s.store.Begin(ctx, BeginUploadRequest{
		UserID:      userID,
		FileName:    req.FileName,
		FileSize:    req.FileSize,
		TotalChunks: int(req.TotalChunks),
		Version:     req.Version,
		Description: req.Description,
	})
	if err != nil {
		return nil, err
	}

	return &pb.UploadBeginResponse{
		UploadId:         session.ID,
		MaxChunkSize:     MaxChunkSize,
		MaxTotalChunks:   MaxTotalChunks,
		MaxAggregateSize: MaxAggregate,
		ExpiresAt:        int32(session.ExpiresAt.Unix()),
		Status:           string(session.Status),
		ChunksReceived:   0,
		BytesReceived:    0,
	}, nil
}

func (s *UploadService) Status(ctx context.Context, uploadID uuid.UUID) (*pb.UploadStatusResponse, error) {
	session, err := s.store.Get(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	return &pb.UploadStatusResponse{
		UploadId:       session.ID,
		Status:         string(session.Status),
		TotalChunks:    int32(session.TotalChunks),
		ChunksReceived: int32(session.ChunksReceived),
		BytesReceived:  session.BytesReceived,
		ExpiresAt:      int32(session.ExpiresAt.Unix()),
		FileName:       session.FileName,
		FileSize:       session.FileSize,
	}, nil
}

func (s *UploadService) PutChunk(ctx context.Context, uploadID uuid.UUID, chunkIndex int, reader io.Reader) (*pb.UploadChunkResponse, error) {
	session, err := s.store.Get(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	if session.Status != StatusReceiving {
		return nil, fmt.Errorf("upload is not in receiving state")
	}
	if chunkIndex < 0 || chunkIndex >= session.TotalChunks {
		return nil, fmt.Errorf("chunk index %d out of range [0, %d)", chunkIndex, session.TotalChunks)
	}

	result, err := s.chunkStore.WriteChunk(session.ID, chunkIndex, reader)
	if err != nil {
		return nil, fmt.Errorf("write chunk: %w", err)
	}

	if err := s.store.RecordChunk(ctx, uploadID, ChunkRecord{
		Index: chunkIndex,
		Hash:  result.Hash,
		Size:  result.ByteSize,
	}); err != nil {
		return nil, err
	}

	return &pb.UploadChunkResponse{
		UploadId:   uploadID.String(),
		ChunkIndex: int32(chunkIndex),
		Ok:         true,
	}, nil
}

func (s *UploadService) Complete(ctx context.Context, uploadID uuid.UUID, userID uint) (*pb.UploadCompleteResponse, error) {
	session, err := s.store.Get(ctx, uploadID)
	if err != nil {
		return nil, err
	}

	if session.UserID != userID {
		return nil, fmt.Errorf("owner mismatch")
	}

	if err := s.store.BeginAssembly(ctx, uploadID, uint(session.TotalChunks)); err != nil {
		return nil, err
	}

	pub := &UploadPublication{
		UploadID: uploadID.String(),
		Status:   "assembling",
		UserID:   userID,
	}
	if err := s.db.Create(pub).Error; err != nil {
		return nil, fmt.Errorf("create publication record: %w", err)
	}

	storagePath := StoragePath()
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	tmpPath := filepath.Join(storagePath, fmt.Sprintf("upload-asm-%s.apk", uploadID.String()))
	outFile, err := os.Create(tmpPath)
	if err != nil {
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("create assembled temp: %w", err)
	}

	var totalSize int64
	for i := 0; i < session.TotalChunks; i++ {
		chunkFile, err := s.chunkStore.ReadChunk(session.ID, i)
		if err != nil {
			outFile.Close()
			os.Remove(tmpPath)
			s.markPublicationFailed(pub.ID)
			return nil, fmt.Errorf("read chunk %d: %w", i, err)
		}
		written, copyErr := io.Copy(outFile, chunkFile)
		chunkFile.Close()
		if copyErr != nil {
			outFile.Close()
			os.Remove(tmpPath)
			s.markPublicationFailed(pub.ID)
			return nil, fmt.Errorf("copy chunk %d: %w", i, copyErr)
		}
		totalSize += written
	}
	outFile.Close()

	apkInfo, parseErr := ParseAPKInfo(tmpPath)
	if parseErr != nil {
		os.Remove(tmpPath)
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("parse APK: %w", parseErr)
	}

	if err := ValidatePackageName(apkInfo.PackageName); err != nil {
		os.Remove(tmpPath)
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("invalid package: %w", err)
	}

	version := session.Version
	if version == "" {
		version = apkInfo.VersionName
	}
	if version == "" || !semverPattern.MatchString(version) {
		os.Remove(tmpPath)
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("invalid version format")
	}

	fileName := fmt.Sprintf("%s-%s.apk", apkInfo.PackageName, version)
	destPath, err := SafeDestination(storagePath, fileName)
	if err != nil {
		os.Remove(tmpPath)
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("invalid destination: %w", err)
	}

	if err := PublishNoReplace(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		s.markPublicationFailed(pub.ID)
		if err.Error() == "file exists" {
			return nil, fmt.Errorf("APK version already exists")
		}
		return nil, fmt.Errorf("publish: %w", err)
	}

	pub.TempPath = tmpPath
	pub.FinalPath = destPath
	pub.Status = "published"
	s.db.Save(pub)

	description := session.Description
	apkV, err := s.repo.Create(
		version, fileName, destPath, description, apkInfo.PackageName,
		totalSize, apkInfo.VersionCode, apkInfo.MinSDKVersion, apkInfo.TargetSDKVersion,
	)
	if err != nil {
		s.markPublicationFailed(pub.ID)
		return nil, fmt.Errorf("create DB record: %w", err)
	}

	pub.Status = "persisted"
	s.db.Save(pub)

	if err := s.store.Complete(ctx, uploadID); err != nil {
		return nil, err
	}

	s.chunkStore.RemoveChunks(session.ID)

	return &pb.UploadCompleteResponse{
		Id:               uint32(apkV.ID),
		Version:          apkV.Version,
		FileName:         apkV.FileName,
		FileSize:         apkV.FileSize,
		Description:      apkV.Description,
		PackageName:      apkV.PackageName,
		VersionCode:      apkV.VersionCode,
		MinSdkVersion:    apkV.MinSDKVersion,
		TargetSdkVersion: apkV.TargetSDKVersion,
		DownloadToken:    apkV.DownloadToken,
		DownloadUrl:      fmt.Sprintf("/api/app/v1/apk/download/%s", apkV.DownloadToken),
		CreatedAt:        apkV.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *UploadService) Abort(ctx context.Context, uploadID uuid.UUID, userID uint) error {
	session, err := s.store.Get(ctx, uploadID)
	if err != nil {
		return err
	}

	if session.UserID != userID {
		return fmt.Errorf("owner mismatch")
	}

	s.chunkStore.RemoveChunks(session.ID)

	var pub UploadPublication
	if err := s.db.Where("upload_id = ?", uploadID.String()).First(&pub).Error; err == nil {
		if pub.TempPath != "" {
			os.Remove(pub.TempPath)
		}
		pub.Status = "failed"
		s.db.Save(&pub)
	}

	return s.store.Abort(ctx, uploadID)
}

func (s *UploadService) markPublicationFailed(pubID uint) {
	s.db.Model(&UploadPublication{}).Where("id = ?", pubID).Update("status", "failed")
}

func (s *UploadService) ReconcilePublications(ctx context.Context) error {
	var pubs []UploadPublication
	if err := s.db.Where("status IN ?", []string{"assembling", "published"}).Find(&pubs).Error; err != nil {
		return err
	}

	for _, pub := range pubs {
		switch pub.Status {
		case "published":
			if pub.FinalPath != "" {
				if _, err := os.Stat(pub.FinalPath); os.IsNotExist(err) {
					s.db.Model(&pub).Update("status", "failed")
					continue
				}
			}
			_, err := s.store.Get(ctx, uuid.MustParse(pub.UploadID))
			if err != nil {
				continue
			}
			s.store.Complete(context.Background(), uuid.MustParse(pub.UploadID))
		case "assembling":
			s.db.Model(&pub).Update("status", "failed")
			if pub.TempPath != "" {
				os.Remove(pub.TempPath)
			}
			s.store.Abort(context.Background(), uuid.MustParse(pub.UploadID))
			s.chunkStore.RemoveChunks(pub.UploadID)
		}
	}

	return nil
}
