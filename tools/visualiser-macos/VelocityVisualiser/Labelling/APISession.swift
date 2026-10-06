import Foundation

/// Shared URLSession policy for the visualiser's HTTP API clients.
///
/// `URLSession.shared` carries Foundation's on-disk `URLCache`, which keeps a
/// SQLite database under `Library/Caches/<bundle-id>/Cache.db`. That is the
/// database behind the `execSQLStatement ... disk I/O error` messages the app
/// logs: the cache is Foundation's, not ours, and the failures come from the
/// container rather than from anything the app asked for.
///
/// Nothing here benefits from HTTP caching. These clients talk to a local
/// labelling API where a cached GET is worse than useless — it can hand back a
/// label set that has already been edited. Using a session with no cache
/// removes the stale-read risk and stops the app writing a cache database it
/// never reads.
enum APISession {

    /// A session that neither reads nor writes the on-disk URL cache.
    static func makeUncached() -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        return URLSession(configuration: config)
    }

    /// The session the API clients use by default.
    static let shared: URLSession = makeUncached()
}
