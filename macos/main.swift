import AppKit
import SwiftUI

// NSPopover creates its window asynchronously. Hide it at attachment, before
// AppKit orders it on screen, so its provisional frame is never visible.
private final class PopoverAttachment: NSView {
    override func viewWillMove(toWindow newWindow: NSWindow?) {
        super.viewWillMove(toWindow: newWindow)
        newWindow?.alphaValue = 0
    }
}

final class Delegate: NSObject, NSApplicationDelegate, NSPopoverDelegate {
    let store = Store()
    let panel = NSPopover()
    var item: NSStatusItem!
    private let badge = Badge()
    private var transition: Timer?
    private var dismissNotice: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        guard let button = item.button else { return }
        // The status button is only 22 pt high even in a taller menu bar.
        // Draw in its window, centered on the button, and keep clicks on the native control.
        let container = button.window?.contentView ?? button
        badge.isBordered = false
        badge.title = ""
        badge.setButtonType(.momentaryPushIn)
        badge.target = self
        badge.action = #selector(toggle)
        badge.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(badge)
        let preferredHeight = badge.heightAnchor.constraint(equalToConstant: 24)
        preferredHeight.priority = .defaultHigh
        NSLayoutConstraint.activate([
            badge.centerXAnchor.constraint(equalTo: button.centerXAnchor),
            badge.centerYAnchor.constraint(equalTo: button.centerYAnchor),
            badge.widthAnchor.constraint(equalTo: button.widthAnchor),
            badge.heightAnchor.constraint(lessThanOrEqualTo: container.heightAnchor),
            preferredHeight
        ])
        item.length = 48
        // Keep the system button for sizing and anchoring only.
        button.isTransparent = true
        button.setAccessibilityElement(false)
        panel.behavior = .transient
        panel.animates = false
        panel.delegate = self
        panel.contentViewController = NSHostingController(rootView: Panel(store: store, openTerminal: { [weak self] in
            self?.openTerminal()
        }))
        panel.contentViewController?.view.addSubview(PopoverAttachment(frame: .zero))
        store.updateCount = { [weak self] count in
            guard let self else { return }
            self.badge.count = count.map(String.init) ?? "–"
            self.updateBadgeLanguage()
        }
        store.portsOpened = { [weak self] in self?.notifyOpened() }
        store.languageChanged = { [weak self] in self?.updateBadgeLanguage() }
        updateBadgeLanguage()
        store.start()
    }

    private func updateBadgeLanguage() {
        badge.language = store.language
        item.length = badge.width
        let label = store.language.text("Ghosts · Local listening ports", "Ghosts · 本機監聽端口")
        badge.toolTip = label
        badge.setAccessibilityLabel(label + " " + badge.count)
    }

    private func notifyOpened() {
        dismissNotice?.invalidate()
        animateBadge(to: 1)
        if badge.window != nil {
            NSAccessibility.post(element: badge, notification: .announcementRequested, userInfo: [
                .announcement: badge.language.greeting, .priority: NSAccessibilityPriorityLevel.medium.rawValue
            ])
        }
        let timer = Timer(timeInterval: 5, repeats: false) { [weak self] _ in self?.animateBadge(to: 0) }
        RunLoop.main.add(timer, forMode: .common)
        dismissNotice = timer
    }

    private func animateBadge(to target: CGFloat) {
        transition?.invalidate()
        let from = badge.expansion
        if NSWorkspace.shared.accessibilityDisplayShouldReduceMotion || from == target {
            badge.expansion = target
            item.length = badge.width
            return
        }
        let start = ProcessInfo.processInfo.systemUptime
        let timer = Timer(timeInterval: 1.0 / 60, repeats: true) { [weak self] timer in
            guard let self else { timer.invalidate(); return }
            let t = min(1, (ProcessInfo.processInfo.systemUptime - start) / 0.3)
            let eased = t * t * (3 - 2 * t)
            self.badge.expansion = from + (target - from) * eased
            self.item.length = self.badge.width
            if t == 1 { timer.invalidate() }
        }
        RunLoop.main.add(timer, forMode: .common)
        transition = timer
    }

    func openTerminal() {
        guard let command = Bundle.main.url(forResource: "Ghosts", withExtension: "command"),
              let terminal = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.apple.Terminal") else {
            store.message = .missingTerminal
            return
        }
        NSWorkspace.shared.open([command], withApplicationAt: terminal, configuration: .init()) { _, error in
            DispatchQueue.main.async {
                if let error {
                    self.store.message = .terminal(error.localizedDescription)
                } else {
                    self.panel.performClose(nil)
                }
            }
        }
    }

    func popoverDidShow(_ notification: Notification) {
        guard let window = panel.contentViewController?.view.window else { return }
        if let button = item.button, let status = button.window, let screen = status.screen {
            let rect = status.convertToScreen(button.convert(button.bounds, to: nil))
            window.setFrameTopLeftPoint(NSPoint(x: window.frame.minX, y: min(rect.minY, screen.visibleFrame.maxY)))
        }
        window.alphaValue = 1
        window.makeKey()
    }

    @objc func toggle() {
        if panel.isShown { panel.performClose(nil); return }
        guard let button = item.button, let content = panel.contentViewController?.view else { return }
        store.refresh()
        panel.contentSize = content.fittingSize
        let anchor = button.bounds.insetBy(dx: 2, dy: 4)
        panel.show(relativeTo: anchor, of: button, preferredEdge: button.isFlipped ? .maxY : .minY)
    }
}

let delegate = Delegate()
let app = NSApplication.shared
app.delegate = delegate
app.run()
