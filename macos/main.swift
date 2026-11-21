import AppKit
import SwiftUI

struct Listener: Decodable, Identifiable {
    let id, name, address, service, title: String
    let memory: Int64
}

final class Store: ObservableObject {
    @Published var rows: [Listener] = []
    @Published var message: String?
    @Published var opening: String?
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
                self.updateCount?(rows.map { Set($0.map(\.address)).count })
                self.scanning = false
                if rows == nil { self.message = "無法讀取本機端口，將自動重試。" }
                else if self.message == "無法讀取本機端口，將自動重試。" { self.message = nil }
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
                    self.message = "無法在 \(row.address) 取得網頁回應。"
                    return
                }
                guard let url = URL(string: "\(scheme == 1 ? "https" : "http")://\(row.address)/"),
                      NSWorkspace.shared.open(url) else {
                    self.message = "無法在預設瀏覽器開啟 \(row.address)。"
                    return
                }
            }
        }
    }
}

private let ghostMark = "👻" // Same mark as the project website's favicon and masthead.

struct ServiceRow: View {
    let row: Listener
    let opening: Bool
    let open: () -> Void
    @State private var hovered = false

    var body: some View {
        Button(action: open) {
            HStack(spacing: 10) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(row.title.isEmpty ? row.name : row.title).fontWeight(.medium)
                        .help(row.title.isEmpty ? row.name : row.title + " · " + row.name)
                    Text(row.address).font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                }.lineLimit(1)
                Spacer(minLength: 4)
                VStack(alignment: .trailing, spacing: 3) {
                    Text(ByteCountFormatter.string(fromByteCount: row.memory, countStyle: .memory))
                        .monospacedDigit()
                    if !row.service.isEmpty {
                        Text(row.service).font(.caption).foregroundStyle(.secondary)
                    }
                }.fixedSize().help("程序與子程序的記憶體用量")
                Image(systemName: opening ? "hourglass" : "arrow.up.right")
                    .font(.caption).foregroundStyle(.secondary)
                    .opacity(row.service.isEmpty ? 1 : 0)
            }
            .padding(.horizontal, 9).frame(height: 46)
            .background(RoundedRectangle(cornerRadius: 7)
                .fill(hovered && row.service.isEmpty ? Color.primary.opacity(0.08) : .clear))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(!row.service.isEmpty)
        .onHover { hovered = $0 }
    }
}

struct Panel: View {
    @ObservedObject var store: Store
    let openTerminal: () -> Void
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 7) {
                Text(ghostMark).accessibilityHidden(true)
                Text("Ghosts").font(.headline)
                Spacer()
                Button("結束") { NSApp.terminate(nil) }
                    .controlSize(.small).help("結束 Ghosts")
            }.padding(.horizontal, 15).padding(.vertical, 9)
            Divider()
            if !store.rows.isEmpty {
                HStack {
                    Text("本機服務")
                    Spacer()
                    Text("記憶體")
                }.font(.caption).foregroundStyle(.secondary)
                    .padding(.leading, 16).padding(.trailing, 34).padding(.top, 9)
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(store.rows) { row in
                            ServiceRow(row: row, opening: store.opening == row.id) { store.open(row) }
                        }
                    }
                }.frame(height: CGFloat(min(store.rows.count, 7)) * 46)
                    .padding(.horizontal, 7).padding(.vertical, 5)
            }
            if let message = store.message {
                Text(message).font(.callout).foregroundStyle(.secondary).padding(14)
            } else if store.rows.isEmpty {
                Text("目前沒有開發服務在監聽端口。")
                    .foregroundStyle(.secondary).padding(16)
            }
            Divider()
            Button(action: openTerminal) {
                Label("開啟完整終端介面", systemImage: "terminal")
            }
            .controlSize(.small)
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
        }.frame(width: 332)
    }
}

// Leave event handling and accessibility with the system status bar button.
final class Badge: NSView {
    var count = "…" { didSet { needsDisplay = true } }
    override func hitTest(_ point: NSPoint) -> NSView? { nil }

    override func draw(_ dirtyRect: NSRect) {
        let height = max(1, bounds.height - 2)
        let pill = NSRect(x: 2, y: (bounds.height - height) / 2,
                          width: bounds.width - 4, height: height)
        NSColor.labelColor.withAlphaComponent(0.13).setFill()
        NSBezierPath(roundedRect: pill, xRadius: height / 2, yRadius: height / 2).fill()
        let title = NSAttributedString(string: ghostMark + " " + count, attributes: [
            .font: NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium),
            .foregroundColor: NSColor.labelColor
        ])
        let size = title.size()
        title.draw(at: NSPoint(x: (bounds.width - size.width) / 2, y: (bounds.height - size.height) / 2))
    }
    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        needsDisplay = true
    }
}

final class Delegate: NSObject, NSApplicationDelegate, NSPopoverDelegate {
    let store = Store()
    let panel = NSPopover()
    var item: NSStatusItem!

    func applicationDidFinishLaunching(_ notification: Notification) {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        guard let button = item.button else { return }
        let badge = Badge(frame: button.bounds)
        badge.autoresizingMask = [.width, .height]
        button.addSubview(badge)
        item.length = 48
        button.toolTip = "Ghosts · 本機監聽端口"
        button.target = self
        button.action = #selector(toggle)
        panel.behavior = .transient
        panel.delegate = self
        panel.contentViewController = NSHostingController(rootView: Panel(store: store, openTerminal: { [weak self] in
            self?.openTerminal()
        }))
        store.updateCount = { count in
            badge.count = count.map(String.init) ?? "–"
            self.item.length = 48 + CGFloat(max(0, badge.count.count - 1)) * 8
            button.setAccessibilityLabel("Ghosts，本機監聽端口 " + badge.count)
        }
        store.start()
    }

    func openTerminal() {
        guard let command = Bundle.main.url(forResource: "Ghosts", withExtension: "command"),
              let terminal = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.apple.Terminal") else {
            store.message = "找不到 Ghosts 終端介面或「終端機」App。"
            return
        }
        NSWorkspace.shared.open([command], withApplicationAt: terminal, configuration: .init()) { _, error in
            DispatchQueue.main.async {
                if let error {
                    self.store.message = "無法開啟終端介面：" + error.localizedDescription
                } else {
                    self.panel.performClose(nil)
                }
            }
        }
    }

    func popoverDidShow(_ notification: Notification) {
        guard let button = item.button, let status = button.window,
              let window = panel.contentViewController?.view.window, let screen = status.screen else { return }
        // The status item can live above the screen's usable frame (e.g. on a notched Mac).
        // Pin the popover's top to the menu bar once AppKit has sized its content.
        let rect = status.convertToScreen(button.convert(button.bounds, to: nil))
        let top = min(rect.minY, screen.visibleFrame.maxY)
        window.setFrameTopLeftPoint(NSPoint(x: window.frame.minX, y: top))
    }

    @objc func toggle() {
        if panel.isShown { panel.performClose(nil); return }
        guard let button = item.button else { return }
        store.refresh()
        let anchor = button.bounds.insetBy(dx: 2, dy: 4)
        panel.show(relativeTo: anchor, of: button, preferredEdge: button.isFlipped ? .maxY : .minY)
        panel.contentViewController?.view.window?.makeKey()
    }
}

let delegate = Delegate()
let app = NSApplication.shared
app.delegate = delegate
app.run()
