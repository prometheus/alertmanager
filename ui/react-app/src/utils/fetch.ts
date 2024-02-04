/**
 * Calls `global.fetch`, but throws a `FetchError` for non-200 responses.
 */
export async function fetch(...args: Parameters<typeof global.fetch>) {
  const response = await global.fetch(...args);
  if (!response.ok) {
    throw new FetchError(response);
  }
  return response;
}

/**
 * Calls `global.fetch` and throws a `FetchError` on non-200 responses, but also
 * decodes the response body as JSON, casting it to type `T`. Returns the
 * decoded body.
 */
export async function fetchJson<T>(...args: Parameters<typeof global.fetch>) {
  const response = await fetch(...args);
  const json: T = await response.json();
  return json;
}

/**
 * Error thrown when fetch returns a non-200 response.
 */
export class FetchError extends Error {
  constructor(readonly response: Response) {
    super(`${response.status} ${response.statusText}`);
    Object.setPrototypeOf(this, FetchError.prototype);
  }
}
