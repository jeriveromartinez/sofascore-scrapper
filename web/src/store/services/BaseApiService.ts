import { API_BASE_URL, KEY_USER_LOGIN } from "../../constants";
import axios, { type AxiosInstance } from "axios";
import { decode, encode } from "cbor-x";
import { type ApiErrorResponse, type UserAuthModel } from "./models";

const CBOR_CONTENT_TYPE = "application/cbor";

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
    const headers: Record<string, string> = { Accept: CBOR_CONTENT_TYPE };
    if (token) headers.Authorization = `Bearer ${token}`;
    if (withBody) headers["Content-Type"] = CBOR_CONTENT_TYPE;

    return headers;
  }

  private decodeResponse<T>(data: ArrayBuffer): T {
    const bytes = new Uint8Array(data);

    if (!bytes.byteLength) return undefined as T;

    return decode(bytes) as T;
  }

  private parseErrorMessage(status: number, data: ArrayBuffer): string {
    const payload = this.decodeResponse<ApiErrorResponse | undefined>(data);

    if (payload && typeof payload === "object" && "error" in payload) {
      return payload.error;
    }

    return `HTTP ${status}`;
  }

  private assertSuccess(status: number, data: ArrayBuffer): void {
    if (status < 400) return;

    throw new Error(this.parseErrorMessage(status, data));
  }

  protected async get<T>(url: string): Promise<T> {
    const headers = this.getHeaders();
    const { data, status } = await this.http.get<ArrayBuffer>(
      `${this.pathApi}${url}`,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }

  protected async post<T, B = unknown>(url: string, body?: B): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = body === undefined ? undefined : encode(body);
    const { data, status } = await this.http.post<ArrayBuffer>(
      `${this.pathApi}${url}`,
      payload,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }

  protected async put<T, B = unknown>(url: string, body?: B): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = body === undefined ? undefined : encode(body);
    const { data, status } = await this.http.put<ArrayBuffer>(
      `${this.pathApi}${url}`,
      payload,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }

  protected async patch<T, B = unknown>(url: string, body?: B): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = body === undefined ? undefined : encode(body);
    const { data, status } = await this.http.patch<ArrayBuffer>(
      `${this.pathApi}${url}`,
      payload,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }

  protected async postMultipart<T>(
    url: string,
    formData: FormData,
  ): Promise<T> {
    const token = this.getToken();
    const headers: Record<string, string> = { Accept: CBOR_CONTENT_TYPE };
    if (token) headers.Authorization = `Bearer ${token}`;

    const { data, status } = await this.http.post<ArrayBuffer>(
      `${this.pathApi}${url}`,
      formData,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }

  /**
   * Send a multipart/form-data request and parse the response as plain JSON.
   * Use this instead of postMultipart when CBOR encoding overhead is undesirable
   * (e.g. chunked file uploads where throughput matters more than payload size).
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
        errData && typeof errData === "object" && typeof errData.error === "string"
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
    if (contentType.includes(CBOR_CONTENT_TYPE)) {
      this.assertSuccess(response.status, response.data);
    } else if (response.status >= 400) {
      throw new Error(`HTTP ${response.status}`);
    }

    return new Blob([response.data], {
      type: contentType || "application/octet-stream",
    });
  }

  protected async delete<T>(url: string): Promise<T> {
    const headers = this.getHeaders();
    const { data, status } = await this.http.delete<ArrayBuffer>(
      `${this.pathApi}${url}`,
      {
        headers,
        responseType: "arraybuffer",
        validateStatus: () => true,
      },
    );

    this.assertSuccess(status, data);

    return this.decodeResponse<T>(data);
  }
}
