const apiPrefix = '/api/v2';

export type URLParams = {
  resource: string;
  queryParams?: URLSearchParams;
  apiPrefix?: string;
};

export default function buildURL(params: URLParams): string {
  let url = params.apiPrefix === undefined ? apiPrefix : params.apiPrefix;
  url = `${url}/${params.resource}`;

  if (params.queryParams !== undefined) {
    url = `${url}?${params.queryParams.toString()}`;
  }
  return url;
}
