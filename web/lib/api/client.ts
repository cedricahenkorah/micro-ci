import axios, { type AxiosRequestConfig } from "axios";
import type {
  ApiErrorResponse,
  ApiResponse,
  Page,
  Run,
  RunDetails,
  RunSummary,
} from "./types";

export const API_URL = (
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:4000/api/v1"
).replace(/\/$/, "");

const http = axios.create({
  baseURL: API_URL,
  timeout: 15_000,
  headers: { Accept: "application/json" },
});

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(config: AxiosRequestConfig): Promise<T> {
  try {
    const response = await http.request<ApiResponse<T>>(config);

    return response.data.data;
  } catch (error) {
    if (axios.isCancel(error)) {
      throw error;
    }

    if (axios.isAxiosError<ApiErrorResponse>(error)) {
      throw new ApiError(
        error.response?.data?.message ||
          (error.response
            ? `Request failed (${error.response.status}).`
            : "Cannot reach the API. Check that the server is running and the frontend origin is allowed."),
        error.response?.status ?? 0,
      );
    }

    throw error;
  }
}

export function listRuns(page = 0, limit = 20, signal?: AbortSignal) {
  return request<Page<RunSummary>>({
    url: "/runs",
    params: { page, limit },
    signal,
  });
}

export function getRun(id: string, signal?: AbortSignal) {
  return request<RunDetails>({
    url: `/runs/${id}`,
    signal,
  });
}

export function cancelRun(id: string) {
  return request<Run>({
    method: "POST",
    url: `/runs/${id}/cancel`,
  });
}

export function retryRun(id: string) {
  return request<Run>({
    method: "POST",
    url: `/runs/${id}/retry`,
  });
}
