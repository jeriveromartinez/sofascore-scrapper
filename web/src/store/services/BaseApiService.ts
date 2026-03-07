import { API_BASE_URL, KEY_USER_LOGIN } from "../../constants";
import axios, { type AxiosInstance } from "axios";
import { decode, encode } from "cbor-x";
import { UserAuthModel } from "./models";

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
    const headers: Record<string, string> = {Accept: CBOR_CONTENT_TYPE};
    if (token) headers.Authorization = `Bearer ${token}`;
    if (withBody) headers["Content-Type"] = CBOR_CONTENT_TYPE;

    return headers;
  }

  private decodeResponse<T>(data: ArrayBuffer): T {
    const bytes = new Uint8Array(data);

    if (!bytes.byteLength) return undefined as T;

    return decode(bytes) as T;
  }

  protected async get<T>(url: string): Promise<T> {
    const headers = this.getHeaders();
    const { data } = await this.http.get<ArrayBuffer>(`${this.pathApi}${url}`, {
      headers,
      responseType: "arraybuffer",
    });

    return this.decodeResponse<T>(data);
  }

  protected async post<T, B = unknown>(url: string, body?: B): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = body === undefined ? undefined : encode(body);
    const { data } = await this.http.post<ArrayBuffer>(
      `${this.pathApi}${url}`,
      payload,
      {
        headers,
        responseType: "arraybuffer",
      },
    );

    return this.decodeResponse<T>(data);
  }

  protected async put<T, B = unknown>(url: string, body?: B): Promise<T> {
    const headers = this.getHeaders(true);
    const payload = body === undefined ? undefined : encode(body);
    const { data } = await this.http.put<ArrayBuffer>(
      `${this.pathApi}${url}`,
      payload,
      {
        headers,
        responseType: "arraybuffer",
      },
    );

    return this.decodeResponse<T>(data);
  }

  protected async delete<T>(url: string): Promise<T> {
    const headers = this.getHeaders();
    const { data } = await this.http.delete<ArrayBuffer>(
      `${this.pathApi}${url}`,
      {
        headers,
        responseType: "arraybuffer",
      },
    );

    return this.decodeResponse<T>(data);
  }
}
