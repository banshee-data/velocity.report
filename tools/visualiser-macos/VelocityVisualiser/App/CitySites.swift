// CitySites.swift
// The city site index, `city/sites.json`, and each scene's
// `assets/placement.json` (lidar-city-view-plan Item 1; the format is
// docs/ui/city-view.md#placement-files). The web city view reads the same
// files.
//
// The visualiser resolves the open site from the index rather than from
// capture file names, and takes the heading from it rather than from a
// typed field. Placement comes only from an accepted pose: a site without
// one has no `sensor_to_city` and is drawn as a marker, never placed.
//
// Matrices are written as nested 4×4 arrays, row-major, metres:
// p_out = M[0:3][0:3] · p_in + M[0:3][3]. simd is column-major, so they are
// transposed on reading.

import Foundation
import simd

enum RowMajor {
    /// A 4×4 matrix from four rows of four numbers; nil unless it is one,
    /// finite, with a last row of [0, 0, 0, 1].
    static func matrix(_ rows: [[Double]]?) -> simd_double4x4? {
        guard let rows, rows.count == 4, rows.allSatisfy({ $0.count == 4 }),
            rows.joined().allSatisfy(\.isFinite), rows[3] == [0, 0, 0, 1]
        else { return nil }
        return simd_double4x4(rows: rows.map { SIMD4($0[0], $0[1], $0[2], $0[3]) })
    }
}

/// How a site's pose was accepted.
enum RegistrationState: String, Sendable {
    case registered
    case manuallyPlaced = "manually_placed"
}

/// One site in `city/sites.json`.
struct CitySite: Identifiable, Hashable, Sendable {
    struct Cells: Hashable, Sendable {
        var l10 = ""
        var l13 = ""
        var l16 = ""
    }

    var id: String
    var title: String
    var published = false
    var cells = Cells()
    /// The scene's web paths: its page, its assets folder and its placement
    /// sidecar, as `/surveys/<id>/…`; nil when the scene is not published.
    var scenePage: String?
    var sceneAssets: String?
    var scenePlacement: String?
    /// Capture paths as the dataset names them; matched by file name.
    var captures: [String] = []
    /// The recording date, San Francisco time ("2026-09-02").
    var recordedOn: String?
    var recordID: String?
    var registrationState: RegistrationState?
    /// The bearing of the sensor's +Y, clockwise from true north: the
    /// heading the alignment hint sends.
    var headingDeg: Double?
    /// Sensor frame to city ENU, from the accepted pose; nil when not placed.
    var sensorToCity: simd_double4x4?
    /// Where the marker goes, city ENU metres, and whether it is the accepted
    /// pose or an eye-read position for a site not yet placed.
    var markerCityENU: SIMD3<Double>?
    var markerIsEyeRead = false
    /// The eye-read position, WGS 84, when the marker gives one.
    var latitude: Double?
    var longitude: Double?

    /// Placed from an accepted pose: the only sites drawn in the city.
    var isPlaced: Bool { sensorToCity != nil }

    /// The sensor's position in the city frame, metres.
    var sensorCityPosition: SIMD3<Double>? {
        guard let m = sensorToCity else { return nil }
        return SIMD3(m.columns.3.x, m.columns.3.y, m.columns.3.z)
    }

    /// The scene viewer's north_azimuth_deg the alignment pane types, from
    /// the heading: the hint turns N into 180 − N (AlignmentSession.hintHeading).
    var northAzimuthDeg: Double? { headingDeg.map { SitePose.wrap360(180 - $0) } }

    static func == (a: CitySite, b: CitySite) -> Bool {
        a.id == b.id && a.title == b.title && a.published == b.published && a.cells == b.cells
            && a.scenePage == b.scenePage && a.sceneAssets == b.sceneAssets
            && a.scenePlacement == b.scenePlacement && a.captures == b.captures
            && a.recordedOn == b.recordedOn && a.recordID == b.recordID
            && a.registrationState == b.registrationState && a.headingDeg == b.headingDeg
            && a.sensorToCity == b.sensorToCity && a.markerCityENU == b.markerCityENU
            && a.markerIsEyeRead == b.markerIsEyeRead && a.latitude == b.latitude
            && a.longitude == b.longitude
    }

    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

/// The marker and placement blocks, as both files write them.
struct PlacementMarker: Decodable, Sendable {
    var source: String?
    var cityEnuM: [Double]?
    /// Present for an eye-read marker.
    var latDeg: Double?
    var lonDeg: Double?

    var position: SIMD3<Double>? {
        guard let p = cityEnuM, p.count == 3, p.allSatisfy(\.isFinite) else { return nil }
        return SIMD3(p[0], p[1], p[2])
    }
}

/// `city/sites.json`.
struct CitySiteIndex: Sendable, Equatable {
    var sites: [CitySite]

    func site(id: String) -> CitySite? { sites.first { $0.id == id } }

    /// The site one of whose captures is `path` (matched by file name).
    func site(capturePath path: String) -> CitySite? {
        let name = (path as NSString).lastPathComponent
        guard !name.isEmpty else { return nil }
        return sites.first { site in
            site.captures.contains { ($0 as NSString).lastPathComponent == name }
        }
    }

    /// The site at an L16 cell, when only one site is there.
    func site(l16Token token: String) -> CitySite? {
        let matches = sites.filter { $0.cells.l16 == token }
        return matches.count == 1 ? matches[0] : nil
    }

    /// The next or previous site in the index's order, wrapping.
    func neighbour(of id: String?, step: Int) -> CitySite? {
        guard !sites.isEmpty else { return nil }
        guard let id, let index = sites.firstIndex(where: { $0.id == id }) else {
            return step >= 0 ? sites.first : sites.last
        }
        let n = sites.count
        return sites[((index + step) % n + n) % n]
    }

    private struct File: Decodable {
        struct Cells: Decodable {
            var l10: String?
            var l13: String?
            var l16: String?
        }
        struct Scene: Decodable {
            var page: String?
            var assets: String?
            var placement: String?
        }
        struct Placement: Decodable {
            var state: String?
            var recordId: String?
            var registrationState: String?
            var headingDeg: Double?
            var sensorToCity: [[Double]]?
            var marker: PlacementMarker?
        }
        struct Site: Decodable {
            var id: String
            var title: String?
            var published: Bool?
            var cells: Cells?
            var scene: Scene?
            var captures: [String]?
            var recordedOn: String?
            var placement: Placement?
        }
        var version: Int?
        var sites: [Site]
    }

    /// Reads the index. Unknown keys are ignored. A site is placed only when
    /// its state says so and its matrix reads.
    static func decode(_ data: Data) throws -> CitySiteIndex {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let file = try decoder.decode(File.self, from: data)
        return CitySiteIndex(
            sites: file.sites.map { s in
                let p = s.placement
                let matrix = p?.state == "placed" ? RowMajor.matrix(p?.sensorToCity) : nil
                return CitySite(
                    id: s.id, title: s.title ?? s.id, published: s.published ?? false,
                    cells: .init(
                        l10: s.cells?.l10 ?? "", l13: s.cells?.l13 ?? "", l16: s.cells?.l16 ?? ""),
                    scenePage: s.scene?.page, sceneAssets: s.scene?.assets,
                    scenePlacement: s.scene?.placement, captures: s.captures ?? [],
                    recordedOn: s.recordedOn, recordID: p?.recordId,
                    registrationState: p?.registrationState.flatMap(RegistrationState.init),
                    headingDeg: p?.headingDeg, sensorToCity: matrix,
                    markerCityENU: p?.marker?.position,
                    markerIsEyeRead: p?.marker?.source == "eye_read", latitude: p?.marker?.latDeg,
                    longitude: p?.marker?.lonDeg)
            })
    }

    /// Reads `city/sites.json` from a directory, a file, or an http(s) URL.
    static func load(
        from location: URL, session: URLSession = APISession.shared
    ) async throws -> CitySiteIndex {
        try decode(await StaticFiles.read(indexURL(for: location), session: session))
    }

    /// A location naming the file itself is used as it is; otherwise the
    /// index is `city/sites.json` beneath it.
    static func indexURL(for location: URL) -> URL {
        if location.pathExtension == "json" { return location }
        return location.appendingPathComponent("city").appendingPathComponent("sites.json")
    }
}

/// A scene's `assets/placement.json`: where its sensor frame sits.
struct ScenePlacement: Sendable, Equatable {
    var siteID: String
    var isPlaced: Bool
    var recordID: String?
    var registrationState: RegistrationState?
    var headingDeg: Double?
    var sensorToSite: simd_double4x4?
    var siteToCity: simd_double4x4?
    var sensorToCity: simd_double4x4?
    /// Why a scene is not placed, as the sidecar says.
    var reason: String?

    private struct File: Decodable {
        struct Matrix: Decodable { var matrix: [[Double]]? }
        struct Record: Decodable {
            var recordId: String?
            var registrationState: String?
        }
        var site: String
        var state: String
        var reason: String?
        var record: Record?
        var headingDeg: Double?
        var sensorToSite: Matrix?
        var siteToCity: Matrix?
        var sensorToCity: Matrix?
    }

    static func decode(_ data: Data) throws -> ScenePlacement {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let f = try decoder.decode(File.self, from: data)
        let sensorToSite = RowMajor.matrix(f.sensorToSite?.matrix)
        let siteToCity = RowMajor.matrix(f.siteToCity?.matrix)
        // Composed when the sidecar leaves the product out.
        var sensorToCity = RowMajor.matrix(f.sensorToCity?.matrix)
        if sensorToCity == nil, let a = siteToCity, let b = sensorToSite { sensorToCity = a * b }
        let placed = f.state == "placed" && sensorToCity != nil
        return ScenePlacement(
            siteID: f.site, isPlaced: placed, recordID: f.record?.recordId,
            registrationState: f.record?.registrationState.flatMap(RegistrationState.init),
            headingDeg: f.headingDeg, sensorToSite: sensorToSite, siteToCity: siteToCity,
            sensorToCity: placed ? sensorToCity : nil, reason: f.reason)
    }
}

/// Reads a static file from disk or over http(s).
enum StaticFiles {
    static func read(_ url: URL, session: URLSession = APISession.shared) async throws -> Data {
        if url.isFileURL { return try Data(contentsOf: url) }
        var request = URLRequest(url: url)
        request.timeoutInterval = 15
        let (data, response) = try await session.data(for: request)
        if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            throw URLError(.badServerResponse)
        }
        return data
    }
}
