import { API_BASE_URL, KEY_USER_LOGIN } from "../../constants";
import { ErrorResponse } from "../../proto/api";
import axios, { type AxiosInstance } from "axios";
import { type ApiErrorResponse, type UserAuthModel } from "./models";

const PROTO_CONTENT_TYPE = "application/x-protobuf";

export interface ProtoCodec<T> {
  encode(message: T): { finish(): Uint8Array };
  decode(input: Uint8Array): T;
}

export abstract class BaseApiService {
  protected readonly http: AxiosInstance;
  protected readonly pathApi: string;

  constructor(pathApi: string) {
    this.http = axios.create({ baseURL: API_BASE_URL });
    this.pathApi = pathApi;
  }

  private getToken(): string {
    const userLogin =
      sessionStorage.getItem(KEY_USER_LOGIN) ??
      localStorage.getItem(KEY_USER_LOGIN) ??
      "{}";

    try {
      const userInfo = JSON.parse(userLogin) as UserAuthModel;

      return userInfo?.token ?? "";
    } catch {
      return "";
    }
  }

  private getHeaders(withBody = false): Record<string, string> {
    const token = this.getToken();
    const headers: Record<string, string> = { Accept: PROTO_CONTENT_TYPE };
    if (token) headers.Authorization = `Bearer ${token}`;
    if (withBody) headers["Content-Type"] = PROTO_CONTENT_TYPE;

    return headers;
  }

  private decodeResponse<T>(data: ArrayBuffer, decoder: ProtoCodec<T>): T {
    const bytes = new Uint8Array(data);

    if (!bytes.byteLength) return undefined as T;

    return decoder.decode(bytes);
  }

  private tryParseJsonError(data: ArrayBuffer): string | undefined {
    const bytes = new Uint8Array(data);
    if (!bytes.byteLength) return undefined;

    try {
      const payload = JSON.parse(new TextDecoder().decode(bytes)) as
        | ApiErrorResponse
        | undefined;
      if (payload && typeof payload.error === "string") {
        return payload.error;
      }
    } catch {
      return undefined;
    }

    return undefined;
  }

  private tryParseProtoError(data: ArrayBuffer): string | undefined {
    const bytes = new Uint8Array(data);
    if (!bytes.byteLength) return undefined;

    try {
      const payload = ErrorResponse.decode(bytes);
      return payload.error || undefined;
    } catch {
      return undefined;
    }
  }

  private parseErrorMessage(
    status: number,
    data: ArrayBuffer,
    contentType: string,
  ): string {
    const jsonError = this.tryParseJsonError(data);
    if (jsonError) return jsonError;

    if (contentType.includes(PROTO_CONTENT_TYPE)) {
      const protoError = this.tryParseProtoError(data);
      if (protoError) return protoError;
    }

    return `HTTP ${status}`;
  }

  private assertSuccess(
    status: number,
    data: ArrayBuffer,
    contentType = "",
  ): void {
    if (status < 400) return;

    throw new Error(this.parseErrorMessage(status, data, contentType));
  }

  private encodeRequest<T>(
    body: T | undefined,
    encoder?: ProtoCodec<T>,
  ): Uint8Array | undefined {
    if (body === undefined || encoder === undefined) return undefined;
    return encoder.encode(body).finish();
  }

  protected async get<T>(url: string, decoder: ProtoCodec<T>): Promise<T> {
    const headers = this.getHeaders();
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.get<ArrayBuffer>(`${this.pathApi}${url}`, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  protected async post<T, B = unknown>(
    url: string,
    body: B | undefined,
    encoder: ProtoCodec<B>,
    decoder: ProtoCodec<T>,
  ): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = this.encodeRequest(body, encoder);
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.post<ArrayBuffer>(`${this.pathApi}${url}`, payload, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
      transformRequest: [(data) => data],
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  protected async put<T, B = unknown>(
    url: string,
    body: B | undefined,
    encoder: ProtoCodec<B>,
    decoder: ProtoCodec<T>,
  ): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = this.encodeRequest(body, encoder);
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.put<ArrayBuffer>(`${this.pathApi}${url}`, payload, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  protected async patch<T, B = unknown>(
    url: string,
    body: B | undefined,
    encoder: ProtoCodec<B>,
    decoder: ProtoCodec<T>,
  ): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = this.encodeRequest(body, encoder);
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.patch<ArrayBuffer>(`${this.pathApi}${url}`, payload, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  protected async postMultipart<T>(
    url: string,
    formData: FormData,
    decoder: ProtoCodec<T>,
  ): Promise<T> {
    const token = this.getToken();
    const headers: Record<string, string> = { Accept: PROTO_CONTENT_TYPE };
    if (token) headers.Authorization = `Bearer ${token}`;

    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.post<ArrayBuffer>(`${this.pathApi}${url}`, formData, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  /**
   * Send a multipart/form-data request and parse the response as plain JSON.
   * Use this instead of postMultipart when the endpoint intentionally responds with JSON
   * (e.g. chunk upload acknowledgements).
   */
  protected async postMultipartJSON<T>(
    url: string,
    formData: FormData,
  ): Promise<T> {
    const token = this.getToken();
    const headers: Record<string, string> = {};
    if (token) headers.Authorization = `Bearer ${token}`;

    const { data, status } = await this.http.post<T>(
      `${this.pathApi}${url}`,
      formData,
      {
        headers,
        validateStatus: () => true,
      },
    );

    if (status >= 400) {
      const errData = data as Record<string, unknown> | undefined;
      const message =
        errData &&
        typeof errData === "object" &&
        typeof errData.error === "string"
          ? errData.error
          : `HTTP ${status}`;
      throw new Error(message);
    }

    return data;
  }

  protected async getBinary(url: string): Promise<Blob> {
    const headers = this.getHeaders();
    const response = await this.http.get<ArrayBuffer>(`${this.pathApi}${url}`, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    const contentType = response.headers["content-type"] ?? "";
    if (
      contentType.includes(PROTO_CONTENT_TYPE) ||
      contentType.includes("application/json")
    ) {
      this.assertSuccess(response.status, response.data, contentType);
    } else if (response.status >= 400) {
      throw new Error(`HTTP ${response.status}`);
    }

    return new Blob([response.data], {
      type: contentType || "application/octet-stream",
    });
  }

  protected async delete<T>(url: string, decoder: ProtoCodec<T>): Promise<T> {
    const headers = this.getHeaders();
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.delete<ArrayBuffer>(`${this.pathApi}${url}`, {
      headers,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }

  protected async deleteWithBody<T, B = unknown>(
    url: string,
    body: B | undefined,
    encoder: ProtoCodec<B>,
    decoder: ProtoCodec<T>,
  ): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = this.encodeRequest(body, encoder);
    const {
      data,
      status,
      headers: responseHeaders,
    } = await this.http.delete<ArrayBuffer>(`${this.pathApi}${url}`, {
      headers,
      data: payload,
      responseType: "arraybuffer",
      validateStatus: () => true,
    });

    this.assertSuccess(status, data, responseHeaders["content-type"] ?? "");

    return this.decodeResponse<T>(data, decoder);
  }
}
