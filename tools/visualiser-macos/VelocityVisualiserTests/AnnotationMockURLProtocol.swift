//
//  AnnotationMockURLProtocol.swift
//  VelocityVisualiserTests
//
//  A URLProtocol stub routed by a unique host per test, rather than one
//  shared static closure.
//
//  The existing MockURLProtocol (see LabelAPIClientTests.swift) has exactly
//  one active handler at a time in a `static var`. Swift Testing runs
//  independent suites concurrently by default, so two tests — anywhere in
//  the target, not only in the same file — can each set that one handler and
//  then await their own request; whichever handler is installed when the
//  request actually starts is the one that answers it. That produced exactly
//  this failure while these tests were being written: a success test
//  decoding a different test's error-only JSON body and failing on a missing
//  "pack_dir" key, non-deterministically.
//
//  Routing by a header carried on the request (the first approach tried
//  here) turned out not to survive every call shape: RunTrackLabelAPIClient
//  reaches some endpoints through `URLSession.data(from: url)` rather than a
//  built `URLRequest`, and a URLSessionConfiguration's httpAdditionalHeaders
//  did not consistently attach to those. The base URL's host does survive
//  every call shape unchanged — it has to, or the request could not be
//  routed anywhere — so each test is given its own unique host instead.
import Foundation

final class AnnotationMockURLProtocol: URLProtocol {
    private static let lock = NSLock()
    private static var handlers: [String: (URLRequest) throws -> (HTTPURLResponse, Data)] = [:]

    /// Returns a session and base URL unique to the caller, plus a function
    /// to register the handler that answers every request made through that
    /// session. No two tests can ever share a host, so no two tests can ever
    /// answer each other's requests, however they happen to be scheduled.
    static func makeSession() -> (
        session: URLSession, baseURL: URL,
        register: (@escaping (URLRequest) throws -> (HTTPURLResponse, Data)) -> Void
    ) {
        let host = "annotation-mock-\(UUID().uuidString).test"
        let baseURL = URL(string: "http://\(host)")!
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [AnnotationMockURLProtocol.self]
        let session = URLSession(configuration: config)
        return (
            session, baseURL,
            { handler in
                lock.lock()
                handlers[host] = handler
                lock.unlock()
            }
        )
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let host = request.url?.host else {
            client?.urlProtocol(
                self,
                didFailWithError: NSError(
                    domain: "AnnotationMockURLProtocol", code: 1,
                    userInfo: [NSLocalizedDescriptionKey: "request has no host to route by"]))
            return
        }
        Self.lock.lock()
        let handler = Self.handlers[host]
        Self.lock.unlock()
        guard let handler else {
            client?.urlProtocol(
                self,
                didFailWithError: NSError(
                    domain: "AnnotationMockURLProtocol", code: 2,
                    userInfo: [
                        NSLocalizedDescriptionKey: "no handler registered for host \(host)"
                    ]))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
