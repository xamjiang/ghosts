import SwiftUI

private enum PanelLayout {
    static let addressFont = NSFont.monospacedSystemFont(ofSize: 12, weight: .medium)
    static let sourceWidth: CGFloat = 74
    static let memoryWidth: CGFloat = 68
    static let actionWidth: CGFloat = 18
    static let rowHeight: CGFloat = 34
}

struct ServiceRow: View {
    let addressWidth: CGFloat
    let language: Language
    let row: Listener
    let opening: Bool
    let open: () -> Void
    @State private var hovered = false

    private var name: String {
        row.title.isEmpty ? (row.service.isEmpty ? row.name : row.service) : row.title
    }

    private var details: some View {
        HStack(spacing: 10) {
            Text(verbatim: row.address)
                .font(Font(PanelLayout.addressFont)).lineLimit(1)
                .padding(.horizontal, 6)
                .frame(width: addressWidth, height: 24, alignment: .leading)
                .background(Color.primary.opacity(0.06), in: RoundedRectangle(cornerRadius: 6))
                .help(row.address)
                .accessibilityLabel(row.address)
            Text(name)
                .fontWeight(.medium).lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
                .help(name + " · " + row.address)
            Text(row.source.isEmpty ? language.text("Unknown", "不明") : row.source)
                .font(.callout).foregroundStyle(.secondary).lineLimit(1)
                .frame(width: PanelLayout.sourceWidth, alignment: .leading)
                .help(row.source.isEmpty ? language.text("Launch source unknown", "啟動來源不明") : row.source)
            Text(ByteCountFormatter.string(fromByteCount: row.memory, countStyle: .memory))
                .monospacedDigit().lineLimit(1).minimumScaleFactor(0.85)
                .frame(width: PanelLayout.memoryWidth, alignment: .trailing)
                .help(language.text("Process and child process memory", "程序與子程序的記憶體用量"))
            Group {
                if opening {
                    ProgressView().controlSize(.small)
                } else if row.service.isEmpty {
                    Image(systemName: "arrow.up.right.square").foregroundStyle(Color.accentColor)
                } else {
                    Color.clear.frame(height: 1)
                }
            }.frame(width: PanelLayout.actionWidth).accessibilityHidden(true)
        }
        .padding(.horizontal, 10).frame(height: PanelLayout.rowHeight)
        .background(RoundedRectangle(cornerRadius: 8)
            .fill(hovered && row.service.isEmpty ? Color.primary.opacity(0.07) : .clear))
        .contentShape(Rectangle())
    }

    var body: some View {
        if row.service.isEmpty {
            Button(action: open) { details }
                .buttonStyle(.plain).onHover { hovered = $0 }
                .accessibilityHint(language.text("Open in your default browser", "在預設瀏覽器開啟"))
        } else {
            details.accessibilityElement(children: .combine)
        }
    }
}

struct Panel: View {
    @ObservedObject var store: Store
    let openTerminal: () -> Void

    var body: some View {
        let addressWidth = store.rows.reduce(CGFloat(116)) { width, row in
            max(width, ceil((row.address as NSString).size(withAttributes: [.font: PanelLayout.addressFont]).width) + 12)
        }
        return VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 8) {
                Text(ghostMark).font(.title2).accessibilityHidden(true)
                Text(store.language.text("Local services", "本機服務")).font(.title3.bold())
                Spacer()
                Picker(store.language.text("Language", "語言"), selection: $store.language) {
                    Text("English").tag(Language.english)
                    Text("繁體中文").tag(Language.chinese)
                }.labelsHidden().fixedSize().controlSize(.small)
            }.padding(.horizontal, 18).padding(.vertical, 12)
            Divider()
            if !store.rows.isEmpty {
                HStack(spacing: 10) {
                    Text(store.language.text("Address", "位址")).padding(.leading, 6)
                        .frame(width: addressWidth, alignment: .leading)
                    Text(store.language.text("Service", "服務")).frame(maxWidth: .infinity, alignment: .leading)
                    Text(store.language.text("Source", "啟動來源"))
                        .frame(width: PanelLayout.sourceWidth, alignment: .leading)
                    Text(store.language.text("Memory", "記憶體"))
                        .frame(width: PanelLayout.memoryWidth, alignment: .trailing)
                    Color.clear.frame(width: PanelLayout.actionWidth, height: 1)
                }.font(.caption.weight(.medium)).foregroundStyle(.secondary)
                    .padding(.horizontal, 18).padding(.top, 10)
                ScrollView {
                    LazyVStack(spacing: 0) {
                        ForEach(store.rows) { row in
                            ServiceRow(addressWidth: addressWidth, language: store.language, row: row, opening: store.opening == row.id) {
                                store.open(row)
                            }
                            .overlay(alignment: .bottom) {
                                if row.id != store.rows.last?.id { Divider().padding(.horizontal, 10) }
                            }
                        }
                    }
                }
                .frame(height: CGFloat(min(store.rows.count, 8)) * PanelLayout.rowHeight)
                .padding(.horizontal, 8).padding(.vertical, 4)
            }
            if let message = store.message {
                Label(message.text(store.language), systemImage: "exclamationmark.circle")
                    .font(.callout).padding(16)
            } else if store.rows.isEmpty {
                Text(store.language.text("No local development services.", "目前沒有開發服務在監聽端口。"))
                    .foregroundStyle(.secondary).frame(maxWidth: .infinity).padding(24)
            }
            Divider()
            HStack {
                Button(action: openTerminal) {
                    Label(store.language.text("Open Ghosts CLI", "開啟 Ghosts CLI"), systemImage: "terminal")
                }
                Spacer()
                Button(store.language.text("Quit Ghosts", "結束 Ghosts")) { NSApp.terminate(nil) }
            }.controlSize(.small).padding(.horizontal, 18).padding(.vertical, 10)
        }.frame(width: 402 + addressWidth)
    }
}
