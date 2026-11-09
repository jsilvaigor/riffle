import CoreGraphics
import Foundation
import Testing
@testable import Riffle

struct TrickplayTests {
    // A 2-hour movie at 5 s: 1440 thumbnails in 15 sheets (server/README.md example).
    let manifest = TrickplayManifest(interval: 5, tileWidth: 320, tileHeight: 180, columns: 10, rows: 10,
                                     count: 1440, sheets: 15, format: "jpg")

    @Test func firstCell() {
        #expect(manifest.cell(at: 0) == .init(index: 0, sheet: 0, rect: CGRect(x: 0, y: 0, width: 320, height: 180)))
    }

    @Test func cellWithinSheet() {
        // k = 23 → row 2, column 3.
        let c = manifest.cell(at: 115)
        #expect(c?.index == 23)
        #expect(c?.sheet == 0)
        #expect(c?.rect == CGRect(x: 960, y: 360, width: 320, height: 180))
    }

    @Test func nextSheetWraps() {
        // k = 100 is the first cell of sheet 1.
        let c = manifest.cell(at: 500)
        #expect(c?.sheet == 1)
        #expect(c?.rect.origin == .zero)
    }

    @Test func roundsToNearestAndClamps() {
        #expect(manifest.cell(at: 7.4)?.index == 1)
        #expect(manifest.cell(at: 7.6)?.index == 2)
        #expect(manifest.cell(at: -3)?.index == 0)
        #expect(manifest.cell(at: 99_999)?.index == 1439)
    }

    @Test func portraitTiles() {
        let m = TrickplayManifest(interval: 1, tileWidth: 90, tileHeight: 160, columns: 10, rows: 10,
                                  count: 30, sheets: 1, format: "jpg")
        #expect(m.cell(at: 12)?.rect == CGRect(x: 180, y: 160, width: 90, height: 160))
    }

    @Test func emptyManifest() {
        let m = TrickplayManifest(interval: 5, tileWidth: 320, tileHeight: 180, columns: 10, rows: 10,
                                  count: 0, sheets: 0, format: "jpg")
        #expect(m.cell(at: 10) == nil)
    }
}

struct DecodingTests {
    @Test func libraryFromServer() throws {
        // Shape of GET /api/library, including Go's null slices and nanosecond times.
        let json = """
        {"roots":["Movies","TV"],"items":[
          {"id":"a1","root":"Movies","path":"Pixar/Up.mkv","name":"Up.mkv","size":4200000000,
           "modTime":"2024-05-01T10:20:30.123456789-03:00",
           "probe":{"v":2,"duration":5730.5,"container":"matroska,webm","bitrate":5000000,
             "video":{"index":0,"codec":"hevc","width":1920,"height":1080,"frameRate":23.976,
                      "rotation":0,"displayWidth":1920,"displayHeight":1080},
             "audio":[{"index":1,"codec":"ac3","channels":6,"language":"eng","default":true}],
             "subtitles":null},
           "trickplay":"ready"},
          {"id":"b2","root":"TV","path":"clip.wmv","name":"clip.wmv","size":10,
           "modTime":"2024-05-01T10:20:30Z","probeError":"invalid data","trickplay":"weird"}
        ]}
        """
        let lib = try RiffleClient.decoder.decode(LibraryResponse.self, from: Data(json.utf8))
        #expect(lib.roots == ["Movies", "TV"])
        #expect(lib.items.count == 2)
        let up = lib.items[0]
        #expect(up.probe?.subtitles.isEmpty == true)
        #expect(up.probe?.audio.first?.channels == 6)
        #expect(up.modTime != nil)
        #expect(up.folderComponents == ["Pixar"])
        #expect(up.displayTitle == "Up")
        #expect(lib.items[1].trickplay == TrickplayState.none)
        #expect(lib.items[1].probe == nil)
    }

    @Test func progressMap() throws {
        let json = #"{"a1":{"position":120.5,"duration":5730.5,"updatedAt":"2026-11-08T12:00:00.5Z"}}"#
        let p = try RiffleClient.decoder.decode([String: WatchProgress].self, from: Data(json.utf8))
        #expect(p["a1"]?.position == 120.5)
        #expect(p["a1"]?.isResumable == true)
    }

    @Test func displayAspectUsesRotation() {
        // Phone clip stored landscape with a 90° flag and no display size (older server).
        let v = VideoStream(index: 0, codec: "h264", width: 1920, height: 1080, rotation: 90)
        #expect((v.displayAspect ?? 0) < 1)
    }
}

struct ClientTests {
    @Test(arguments: [
        ("tower:8770", "http://tower:8770"),
        ("tower", "http://tower:8770"),
        ("192.168.1.10", "http://192.168.1.10:8770"),
        ("http://tower:9000/", "http://tower:9000"),
        ("  https://riffle.tail1234.ts.net  ", "https://riffle.tail1234.ts.net"),
    ])
    func normalize(input: String, expected: String) {
        #expect(RiffleClient.normalize(input)?.absoluteString == expected)
    }

    @Test func rejectsGarbage() {
        #expect(RiffleClient.normalize("") == nil)
        #expect(RiffleClient.normalize("ftp://tower") == nil)
    }
}

struct FormatTests {
    @Test func times() {
        #expect(Format.time(0) == "0:00")
        #expect(Format.time(247) == "4:07")
        #expect(Format.time(3877) == "1:04:37")
        #expect(Format.remaining(760) == "\u{2212}12:40")
        #expect(Format.time(.nan) == "0:00")
    }

    @Test func resolution() {
        let uhd = VideoStream(index: 0, codec: "hevc", width: 3840, height: 2160)
        let portrait = VideoStream(index: 0, codec: "h264", width: 1920, height: 1080,
                                   rotation: 90, displayWidth: 1080, displayHeight: 1920)
        let scope = VideoStream(index: 0, codec: "h264", width: 1920, height: 800)
        #expect(Format.resolution(uhd) == "4K")
        #expect(Format.resolution(portrait) == "1080p")
        #expect(Format.resolution(scope) == "1080p")
    }

    @Test func badgesSkipEverydayCodec() {
        var item = Item(id: "x", root: "Movies", path: "a.mkv", name: "a.mkv", size: 1, trickplay: .ready)
        item.probe = Probe(duration: 60, video: VideoStream(index: 0, codec: "h264", width: 1280, height: 720))
        #expect(Format.badges(item, limit: 2) == ["720p"])
        item.probe?.video?.codec = "hevc"
        #expect(Format.badges(item, limit: 2) == ["720p", "HEVC"])
    }
}

@MainActor
struct LibraryTreeTests {
    @Test func foldersFirstThenVideos() {
        func item(_ root: String, _ path: String) -> Item {
            Item(id: path, root: root, path: path, name: String(path.split(separator: "/").last!),
                 size: 1, trickplay: .none)
        }
        let lib = LibraryResponse(roots: ["Movies", "TV"], items: [
            item("Movies", "b.mkv"), item("Movies", "A.mp4"),
            item("Movies", "Pixar/Up.mkv"), item("Movies", "Pixar/Shorts/Bao.mp4"),
            item("Movies", "Anime/Ghost.mkv"),
        ])
        let roots = Folder.roots(from: lib)
        #expect(roots.map(\.name) == ["Movies", "TV"])
        let movies = roots[0]
        #expect(movies.count == 5)
        #expect(movies.folders.map(\.name) == ["Anime", "Pixar"])
        #expect(movies.videos.map(\.name) == ["A.mp4", "b.mkv"])
        let pixar = movies.folders[1]
        #expect(pixar.id == "Movies/Pixar")
        #expect(pixar.count == 2)
        #expect(pixar.folders.first?.videos.map(\.name) == ["Bao.mp4"])
        #expect(roots[1].count == 0)
    }
}

struct CropTests {
    // 2.40:1 film letterboxed in a 1920x1080 frame; 320x180 tiles.
    let crop = TrickplayCrop(x: 0, y: 140, w: 1920, h: 800, frameWidth: 1920, frameHeight: 1080)
    var manifest: TrickplayManifest {
        TrickplayManifest(interval: 5, tileWidth: 320, tileHeight: 180, columns: 10, rows: 10,
                          count: 100, sheets: 1, format: "jpg", crop: crop)
    }

    @Test func decodesFromManifest() throws {
        let json = #"{"v":2,"interval":5,"tileWidth":320,"tileHeight":180,"columns":10,"rows":10,"count":1,"sheets":1,"format":"jpg","crop":{"x":0,"y":140,"w":1920,"h":800,"frameWidth":1920,"frameHeight":1080}}"#
        let m = try RiffleClient.decoder.decode(TrickplayManifest.self, from: Data(json.utf8))
        #expect(m.crop == crop)
        #expect(m.crop?.mpvValue == "1920x800+0+140")
    }

    @Test func manifestWithoutCrop() throws {
        let json = #"{"interval":5,"tileWidth":320,"tileHeight":180,"columns":10,"rows":10,"count":1,"sheets":1,"format":"jpg"}"#
        let m = try RiffleClient.decoder.decode(TrickplayManifest.self, from: Data(json.utf8))
        #expect(m.crop == nil)
        #expect(m.previewAspect == 320.0 / 180.0)
    }

    @Test func cellLeavesBandsOff() {
        // Cell 11: row 1, column 1. Bands are 140/1080 of 180 px ≈ 23.3 px top and
        // bottom, rounded inwards.
        let c = manifest.cell(at: 55)
        #expect(c?.rect == CGRect(x: 320, y: 180 + 24, width: 320, height: 132))
    }

    @Test func previewAspectIsTheFilm() {
        #expect(abs((manifest.previewAspect ?? 0) - 2.4) < 0.01)
    }
}

struct WatchStateTests {
    @Test func decodesServerEntry() throws {
        let json = #"{"a":{"position":5300,"duration":5400,"updatedAt":"2026-11-08T20:00:00Z","state":"watched","watched":true,"finishedAt":"2026-11-08T20:00:00Z","timesFinished":2},"b":{"position":600,"duration":5400,"state":"partial"},"c":{"position":3,"duration":60,"state":"bogus"}}"#
        let p = try RiffleClient.decoder.decode([String: WatchProgress].self, from: Data(json.utf8))
        #expect(p["a"]?.state == .watched)
        #expect(p["a"]?.timesFinished == 2)
        #expect(p["a"]?.isInProgress == false)
        #expect(p["b"]?.state == .partial)
        #expect(p["b"]?.isInProgress == true)
        #expect(p["c"]?.state == .unwatched)
    }

    @Test func olderServerWithoutState() throws {
        let json = #"{"position":120,"duration":5400}"#
        let p = try RiffleClient.decoder.decode(WatchProgress.self, from: Data(json.utf8))
        #expect(p.state == .partial)
    }

    @Test func rewatchingAWatchedVideoIsInProgress() {
        let p = WatchProgress(position: 900, duration: 5400, state: .watched)
        #expect(p.isInProgress)
    }

    @Test func summaries() {
        #expect(WatchCounts(watched: 14, partial: 2, unwatched: 8).summary == "14 watched · 2 in progress · 8 not watched")
        #expect(WatchCounts(watched: 3, partial: 0, unwatched: 1).summary == "3 watched · 1 not watched")
        #expect(WatchCounts(watched: 5).summary == "All watched")
        #expect(WatchCounts(unwatched: 5).summary == "Not watched")
        #expect(WatchCounts().summary == "No videos")
    }
}
