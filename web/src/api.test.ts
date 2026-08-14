import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchHealth, fetchReview, fetchReviews, triggerReview } from './api';

const mockFetch = vi.fn();

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch);
  mockFetch.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchHealth', () => {
  it('returns true when the server responds ok', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true });
    await expect(fetchHealth()).resolves.toBe(true);
    expect(mockFetch).toHaveBeenCalledWith('/health');
  });

  it('returns false when the request fails', async () => {
    mockFetch.mockRejectedValueOnce(new Error('network down'));
    await expect(fetchHealth()).resolves.toBe(false);
  });
});

describe('fetchReviews', () => {
  it('parses a JSON array', async () => {
    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => [{ id: 1 }] });
    await expect(fetchReviews()).resolves.toEqual([{ id: 1 }]);
    expect(mockFetch).toHaveBeenCalledWith('/api/reviews');
  });

  it('throws on a non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 500 });
    await expect(fetchReviews()).rejects.toThrow('HTTP 500');
  });
});

describe('triggerReview', () => {
  it('POSTs the trigger request as JSON', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ review_id: 1, status: 'started' }),
    });
    const result = await triggerReview({ owner: 'a', repo: 'b', pr_number: 1 });
    expect(result).toEqual({ review_id: 1, status: 'started' });
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/reviews',
      expect.objectContaining({
        method: 'POST',
        body: '{"owner":"a","repo":"b","pr_number":1}',
      }),
    );
  });
});

describe('fetchReview', () => {
  it('throws on a non-ok response', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
    await expect(fetchReview(1)).rejects.toThrow('HTTP 404');
  });
});
