const API_PREFIX = '/api/v2';

export type URLParams = {
  resource: string;
  queryParams?: URLSearchParams;
  apiPrefix?: string;
};

export default function buildURL({ apiPrefix = API_PREFIX, resource, queryParams }: URLParams): string {
  let url = `${apiPrefix}/${resource}`;
  if (queryParams !== undefined) {
    url = `${url}?${queryParams.toString()}`;
  }
  return url;
}
