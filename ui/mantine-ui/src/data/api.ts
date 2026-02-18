import { QueryKey, useQuery, useSuspenseQuery } from '@tanstack/react-query';

// TODO(@sysadmind): Infer this from the current location.
// We don't have a good strategy for storing global settings yet.
const pathPrefix = '';
export const API_PATH = 'api/v2';

type APIError = {
  status: 'error';
  error?: string;
  errorType?: string;
};

type APISuccess<T> = {
  status: 'success';
  data: T;
};

export type APIResponse<T> = APISuccess<T> | APIError;

const isAPIEnvelope = <T>(value: unknown): value is APIResponse<T> => {
  return (
    typeof value === 'object' &&
    value !== null &&
    'status' in value &&
    ((value as { status?: unknown }).status === 'success' ||
      (value as { status?: unknown }).status === 'error')
  );
};

const createQueryFn =
  <T>({
    pathPrefix,
    path,
    params,
    recordResponseTime,
  }: {
    pathPrefix: string;
    path: string;
    params?: Record<string, string | string[]>;
    recordResponseTime?: (time: number) => void;
  }) =>
  async ({ signal }: { signal: AbortSignal }) => {
    const queryParams = new URLSearchParams();
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (Array.isArray(value)) {
          value.forEach((v) => queryParams.append(key, v));
        } else {
          queryParams.set(key, value);
        }
      });
    }
    const queryString = params ? `?${queryParams.toString()}` : '';

    try {
      const startTime = Date.now();

      const res = await fetch(`${pathPrefix}/${API_PATH}${path}${queryString}`, {
        cache: 'no-store',
        credentials: 'same-origin',
        signal,
      });

      if (!res.ok && !res.headers.get('content-type')?.startsWith('application/json')) {
        // For example, Alertmanager may send a 503 Service Unavailable response
        // with a "text/plain" content type when it's starting up. But the API
        // may also respond with a JSON error message and the same error code.
        throw new Error(res.statusText);
      }

      const parsed = await res.json();

      if (recordResponseTime) {
        recordResponseTime(Date.now() - startTime);
      }

      if (isAPIEnvelope<T>(parsed)) {
        if (parsed.status === 'error') {
          throw new Error(
            parsed.error !== undefined ? parsed.error : 'missing "error" field in response JSON'
          );
        }

        return parsed.data;
      }

      return parsed as T;
    } catch (error) {
      if (!(error instanceof Error)) {
        throw new Error('Unknown error');
      }

      switch (error.name) {
        case 'TypeError':
          throw new Error('Network error or unable to reach the server');
        case 'SyntaxError':
          throw new Error('Invalid JSON response');
        default:
          throw error;
      }
    }
  };

type QueryOptions = {
  key?: QueryKey;
  path: string;
  params?: Record<string, string | string[]>;
  enabled?: boolean;
  refetchInterval?: false | number;
  recordResponseTime?: (time: number) => void;
};

export const useAPIQuery = <T>({
  key,
  path,
  params,
  enabled,
  refetchInterval,
  recordResponseTime,
}: QueryOptions) => {
  return useQuery<T>({
    queryKey: key ?? [API_PATH, path, params],
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval,
    gcTime: 0,
    enabled,
    queryFn: createQueryFn<T>({ pathPrefix, path, params, recordResponseTime }),
  });
};

export const useSuspenseAPIQuery = <T>({ key, path, params }: QueryOptions) => {
  return useSuspenseQuery<T>({
    queryKey: key !== undefined ? key : [path, params],
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
    queryFn: createQueryFn({ pathPrefix, path, params }),
  });
};
