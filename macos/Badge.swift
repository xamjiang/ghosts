import AppKit

let ghostMark = "👻" // Same mark as the project website's favicon and masthead.

// Track presses with AppKit, but draw the feedback inside the visible capsule.
private final class BadgeCell: NSButtonCell {
    override func highlight(_ flag: Bool, withFrame frame: NSRect, in view: NSView) {
        isHighlighted = flag
        view.needsDisplay = true
    }
}

final class Badge: NSButton {
    override class var cellClass: AnyClass? {
        get { BadgeCell.self }
        set {}
    }
    var count = "…" { didSet { needsDisplay = true } }
    var expansion: CGFloat = 0 { didSet { needsDisplay = true } }
    // Sampled from the microphone capsule in the reference Display P3 screenshot.
    static let alertOrange = NSColor(displayP3Red: 241/255, green: 162/255, blue: 76/255, alpha: 1)
    var language: Language = .english { didSet { needsDisplay = true } }
    private var greetingWidth: CGFloat {
        ceil((language.greeting as NSString).size(withAttributes: [.font: NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium)]).width) + 9
    }
    var width: CGFloat { 48 + CGFloat(max(0, count.count - 1)) * 8 + greetingWidth * expansion }

    override func draw(_ dirtyRect: NSRect) {
        let height = min(24, bounds.height)
        let pill = NSRect(x: 2, y: (bounds.height - height) / 2,
                          width: bounds.width - 4, height: height)
        let shape = NSBezierPath(roundedRect: pill, xRadius: height / 2, yRadius: height / 2)
        NSColor.labelColor.withAlphaComponent(0.13).setFill()
        shape.fill()
        if expansion > 0 {
            Self.alertOrange.withAlphaComponent(expansion).setFill()
            shape.fill()
        }
        if isHighlighted {
            NSColor.labelColor.withAlphaComponent(0.20).setFill()
            shape.fill()
        }
        let ink = NSColor.labelColor.blended(withFraction: expansion, of: .black) ?? .labelColor
        let font = NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium)
        func drawText(_ text: String, at x: CGFloat, opacity: CGFloat = 1) {
            let label = NSAttributedString(string: text, attributes: [
                .font: font, .foregroundColor: ink.withAlphaComponent(opacity)
            ])
            label.draw(at: NSPoint(x: x, y: (bounds.height - label.size().height) / 2))
        }
        drawText(ghostMark, at: 9)
        if expansion > 0 {
            NSGraphicsContext.saveGraphicsState()
            NSBezierPath(rect: NSRect(x: 28, y: 0, width: greetingWidth * expansion, height: bounds.height)).addClip()
            drawText(language.greeting, at: 28, opacity: expansion)
            NSGraphicsContext.restoreGraphicsState()
        }
        drawText(count, at: 28 + greetingWidth * expansion)
    }
    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        needsDisplay = true
    }
}
