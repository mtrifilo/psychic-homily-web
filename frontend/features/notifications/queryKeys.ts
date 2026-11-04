/**
 * Prefix of every notification-log cache entry: the bell's, the inbox's and
 * the signed-in home's "From your library" read. Mark-read invalidates by this
 * prefix, so an entry built under any other key would keep stale unread dots.
 *
 * ISOMORPHIC: read by the client hooks and by the server prefetch.
 */
export const NOTIFICATION_LOG_KEY = ['notifications', 'log'] as const
