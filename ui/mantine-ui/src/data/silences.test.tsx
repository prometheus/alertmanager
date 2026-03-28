import React, { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useSilence, useSilences } from './silences';

// Error boundary for capturing and testing error states in hooks
// (useSuspenseQuery throws errors that must be caught by an error boundary)
class ErrorBoundary extends React.Component<
  { children: ReactNode; onError?: (error: Error) => void },
  { hasError: boolean; error: Error | null }
> {
  constructor(props: { children: ReactNode; onError?: (error: Error) => void }) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error) {
    if (this.props.onError) {
      this.props.onError(error);
    }
  }

  render() {
    if (this.state.hasError) {
      return null;
    }
    return this.props.children;
  }
}

// Mock data matching the Alertmanager API specification from api/v2/silences endpoint
const mockSilence = {
  comment: 'test',
  createdBy: 'Test User',
  endsAt: '2026-03-28T20:00:33.992Z',
  id: '4a1f2ba3-2d27-45ac-bcff-cb5cf04d7b68',
  matchers: [
    {
      isEqual: true,
      isRegex: false,
      name: 'alertname',
      value: 'alert_annotate',
    },
    {
      isEqual: true,
      isRegex: false,
      name: 'severity',
      value: 'critical',
    },
  ],
  startsAt: '2026-03-28T18:00:38.093Z',
  status: {
    state: 'active' as const,
  },
  updatedAt: '2026-03-28T18:00:38.093Z',
};

describe('Silence API Hooks', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.clearAllMocks();
  });

  afterEach(() => {
    queryClient.clear();
  });

  const getWrapper = (client: QueryClient) => {
    return ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  };

  describe('useSilences - fetch all silences', () => {
    it('should fetch and return array of silences with correct data structure', async () => {
      // Mock the API endpoint
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(JSON.stringify([mockSilence]), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      const { result } = renderHook(() => useSilences(), {
        wrapper: getWrapper(queryClient),
      });

      // Wait for the hook to resolve
      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // Verify data is returned correctly
      expect(result.current.data).toEqual([mockSilence]);
      expect(Array.isArray(result.current.data)).toBe(true);

      // Verify correct API endpoint was called
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('/api/v2/silences'),
        expect.any(Object)
      );
    });

    it('should handle empty response', async () => {
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      const { result } = renderHook(() => useSilences(), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual([]);
    });

    it('should handle API errors (e.g., server returns error status)', async () => {
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            error: 'Internal server error',
            status: 'error',
          }),
          {
            status: 200,
            headers: { 'content-type': 'application/json' },
          }
        )
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilences(), { wrapper });

      // Error boundary should catch the error thrown by the hook
      await waitFor(() => {
        expect(errorCallback).toHaveBeenCalled();
      });

      expect(errorCallback).toHaveBeenCalledWith(
        expect.objectContaining({
          message: expect.stringContaining('Internal server error'),
        })
      );
    });

    it('should handle network errors (e.g., fetch fails)', async () => {
      const mockFetch = vi.fn().mockRejectedValueOnce(new TypeError('Failed to fetch'));
      global.fetch = mockFetch as unknown as typeof fetch;

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilences(), { wrapper });

      await waitFor(() => {
        expect(errorCallback).toHaveBeenCalled();
      });
    });
  });

  describe('useSilence - fetch single silence by ID', () => {
    const silenceId = '4a1f2ba3-2d27-45ac-bcff-cb5cf04d7b68';

    it('should fetch and return a single silence with correct structure', async () => {
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(JSON.stringify(mockSilence), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      const { result } = renderHook(() => useSilence(silenceId), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // Verify returned data has all required Silence properties
      expect(result.current.data).toEqual(mockSilence);
      expect(result.current.data).toHaveProperty('id');
      expect(result.current.data).toHaveProperty('status');
      expect(result.current.data).toHaveProperty('matchers');

      // Verify correct endpoint was called with the ID
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining(`/api/v2/silence/${silenceId}`),
        expect.any(Object)
      );
    });

    it('should handle different silence IDs correctly', async () => {
      const customId = 'custom-silence-id-123';
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(JSON.stringify({ ...mockSilence, id: customId }), {
          status: 200,
          headers: { 'content-type': 'application/json' },
        })
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      renderHook(() => useSilence(customId), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalled();
      });

      // Verify the custom ID was used in the API call
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining(`/api/v2/silence/${customId}`),
        expect.any(Object)
      );
    });

    it('should handle errors when fetching single silence', async () => {
      const mockFetch = vi.fn().mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            error: 'Silence not found',
            status: 'error',
          }),
          {
            status: 200,
            headers: { 'content-type': 'application/json' },
          }
        )
      );
      global.fetch = mockFetch as unknown as typeof fetch;

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilence(silenceId), { wrapper });

      await waitFor(() => {
        expect(errorCallback).toHaveBeenCalled();
      });

      expect(errorCallback).toHaveBeenCalledWith(
        expect.objectContaining({
          message: expect.stringContaining('Silence not found'),
        })
      );
    });

    it('should create separate cache entries for different IDs', async () => {
      const id1 = 'id-1';
      const id2 = 'id-2';

      const mockFetch = vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ ...mockSilence, id: id1 }), {
            status: 200,
            headers: { 'content-type': 'application/json' },
          })
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ ...mockSilence, id: id2 }), {
            status: 200,
            headers: { 'content-type': 'application/json' },
          })
        );
      global.fetch = mockFetch as unknown as typeof fetch;

      // Each query client maintains separate cache per ID
      const client1 = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const client2 = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });

      renderHook(() => useSilence(id1), { wrapper: getWrapper(client1) });
      renderHook(() => useSilence(id2), { wrapper: getWrapper(client2) });

      await waitFor(() => {
        expect(mockFetch.mock.calls.length).toBeGreaterThanOrEqual(2);
      });

      // Verify both endpoints were called
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining(`/api/v2/silence/${id1}`),
        expect.any(Object)
      );
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining(`/api/v2/silence/${id2}`),
        expect.any(Object)
      );

      client1.clear();
      client2.clear();
    });
  });
});
