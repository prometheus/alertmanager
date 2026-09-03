import { createConnectTransport } from '@connectrpc/connect-web';

export const getAPIBaseURL = (baseURI: string | URL = document.baseURI) =>
  new URL('../api', baseURI).toString();

export const createAPITransport = (
  baseURI: string | URL = document.baseURI,
  fetch?: typeof globalThis.fetch
) =>
  createConnectTransport({
    baseUrl: getAPIBaseURL(baseURI),
    useHttpGet: true,
    ...(fetch === undefined ? {} : { fetch }),
  });

export const apiTransport = createAPITransport();
