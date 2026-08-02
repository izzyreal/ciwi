import AppKit
import Foundation

let arguments = CommandLine.arguments
guard arguments.count == 3 else {
    fputs("usage: create-dmg-background.swift <logo_png> <output_png>\n", stderr)
    exit(1)
}

let logoURL = URL(fileURLWithPath: arguments[1])
let outputURL = URL(fileURLWithPath: arguments[2])
guard let logo = NSImage(contentsOf: logoURL) else {
    fputs("could not load ciwi logo\n", stderr)
    exit(1)
}

let width = 780
let height = 440
guard let bitmap = NSBitmapImageRep(
    bitmapDataPlanes: nil,
    pixelsWide: width,
    pixelsHigh: height,
    bitsPerSample: 8,
    samplesPerPixel: 4,
    hasAlpha: true,
    isPlanar: false,
    colorSpaceName: .deviceRGB,
    bytesPerRow: 0,
    bitsPerPixel: 0
), let context = NSGraphicsContext(bitmapImageRep: bitmap) else {
    fputs("could not create DMG background bitmap\n", stderr)
    exit(1)
}

NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = context
let canvas = NSRect(x: 0, y: 0, width: width, height: height)
let gradient = NSGradient(colors: [
    NSColor(calibratedRed: 0.91, green: 0.97, blue: 0.94, alpha: 1),
    NSColor(calibratedRed: 0.71, green: 0.86, blue: 0.78, alpha: 1),
    NSColor(calibratedRed: 0.96, green: 0.91, blue: 0.66, alpha: 1),
])!
gradient.draw(in: canvas, angle: 18)

let glow = NSGradient(starting: NSColor.white.withAlphaComponent(0.76), ending: NSColor.white.withAlphaComponent(0))!
glow.draw(in: NSRect(x: 220, y: 40, width: 340, height: 340), relativeCenterPosition: .zero)

let logoWidth: CGFloat = 300
let logoHeight = logoWidth * logo.size.height / logo.size.width
logo.draw(
    in: NSRect(x: (CGFloat(width) - logoWidth) / 2, y: (CGFloat(height) - logoHeight) / 2 + 12, width: logoWidth, height: logoHeight),
    from: NSRect(origin: .zero, size: logo.size),
    operation: .sourceOver,
    fraction: 0.24
)
context.flushGraphics()
NSGraphicsContext.restoreGraphicsState()

guard let data = bitmap.representation(using: .png, properties: [:]) else {
    fputs("could not encode DMG background\n", stderr)
    exit(1)
}
try data.write(to: outputURL)

