import AppKit
import SwiftUI

struct Listener: Decodable, Identifiable {
    let id, name, address, service, title, source: String
    let memory: Int64
    let newPort: Bool
}

enum Language: String {
    case english = "en", chinese = "zh-Hant"

    func text(_ en: String, _ zh: String) -> String { self == .english ? en : zh }
    var greeting: String { text("Ghost spotted!", "撞著鬼！") }
}

enum PanelMessage: Equatable {
    case scan, noWeb(String), browser(String), missingTerminal, terminal(String)
    func text(_ lang: Language) -> String {
        switch self {
        case .scan: return lang.text("Can’t read local ports. Retrying…", "無法讀取本機端口，將自動重試。")
        case .noWeb(let address): return lang.text("No web response from " + address, "無法在 " + address + " 取得網頁回應。")
        case .browser(let address): return lang.text("Can’t open " + address + " in your browser.", "無法在預設瀏覽器開啟 " + address + "。")
        case .missingTerminal: return lang.text("Can’t find Terminal or the bundled Ghosts CLI.", "找不到 Ghosts 終端介面或「終端機」App。")
        case .terminal(let reason): return lang.text("Can’t open Terminal: ", "無法開啟終端介面：") + reason
        }
    }
}

final class Store: ObservableObject {
    @Published var rows: [Listener] = []
    @Published var language = Language(rawValue: UserDefaults.standard.string(forKey: "interfaceLanguage") ?? "") ?? .english {
        didSet {
            UserDefaults.standard.set(language.rawValue, forKey: "interfaceLanguage")
            languageChanged?()
        }
    }
    var languageChanged: (() -> Void)?
    @Published var message: PanelMessage?
    @Published var opening: String?
    var portsOpened: (() -> Void)?
    var updateCount: ((Int?) -> Void)?
    private var scanning = false
    private var timer: Timer?

    func start() {
        refresh()
        let timer = Timer(timeInterval: 3, repeats: true) { [weak self] _ in self?.refresh() }
        timer.tolerance = 0.5
        RunLoop.main.add(timer, forMode: .common)
        self.timer = timer
    }

    func refresh() {
        guard !scanning else { return }
        scanning = true
        DispatchQueue.global(qos: .utility).async {
            let rows: [Listener]? = {
                guard let raw = ghostsSnapshot() else { return nil }
                defer { free(raw) }
                return try? JSONDecoder().decode([Listener].self, from: Data(bytes: raw, count: strlen(raw)))
            }()
            DispatchQueue.main.async {
                self.rows = rows ?? []
                if rows?.contains(where: \.newPort) == true {
                    self.portsOpened?()
                }
                self.updateCount?(rows.map { Set($0.map(\.address)).count })
                self.scanning = false
                if rows == nil { self.message = .scan }
                else if self.message == .scan { self.message = nil }
            }
        }
    }

    func open(_ row: Listener) {
        guard opening == nil else { return }
        opening = row.id
        message = nil
        DispatchQueue.global(qos: .userInitiated).async {
            let scheme = row.address.withCString { ghostsWebScheme(UnsafeMutablePointer(mutating: $0)) }
            DispatchQueue.main.async {
                self.opening = nil
                guard scheme != 0 else {
                    self.message = .noWeb(row.address)
                    return
                }
                guard let url = URL(string: "\(scheme == 1 ? "https" : "http")://\(row.address)/"),
                      NSWorkspace.shared.open(url) else {
                    self.message = .browser(row.address)
                    return
                }
            }
        }
    }
}
