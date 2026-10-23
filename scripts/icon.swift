// Adapt the selected ImageGen artwork for the Dock; the menu bar is drawn in Go.
// Run from anywhere: swift scripts/icon.swift
import AppKit

let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
let sourceURL = root.appendingPathComponent("scripts/icon-selected-fox.png")
guard let source = NSBitmapImageRep(data: try Data(contentsOf: sourceURL)),
      source.bitsPerSample == 8, !source.isPlanar, source.samplesPerPixel >= 3,
      let pixels = source.bitmapData else {
	fatalError("icon-selected-fox.png must be an 8-bit RGB image")
}
let stride = source.bitsPerPixel / 8
let colorOffset = source.bitmapFormat.contains(.alphaFirst) ? 1 : 0
func light(_ x: Int, _ y: Int) -> Double {
	let i = y * source.bytesPerRow + x * stride + colorOffset
	return Double(Int(pixels[i]) + Int(pixels[i+1]) + Int(pixels[i+2])) / 765
}

// The selected artwork has a black surround outside its white tile. Search
// inside that tile, then retain the exact dark mark, including the ear cutouts.
let xRange = Int(Double(source.pixelsWide) * 0.18)..<Int(Double(source.pixelsWide) * 0.82)
let yRange = Int(Double(source.pixelsHigh) * 0.18)..<Int(Double(source.pixelsHigh) * 0.82)
var minX = xRange.upperBound, minY = yRange.upperBound, maxX = 0, maxY = 0
for y in yRange {
	for x in xRange where light(x, y) < 0.5 {
		minX = min(minX, x); maxX = max(maxX, x)
		minY = min(minY, y); maxY = max(maxY, y)
	}
}
guard minX > xRange.lowerBound + 2, minY > yRange.lowerBound + 2,
      maxX < xRange.upperBound - 3, maxY < yRange.upperBound - 3,
      minX < maxX, minY < maxY else {
	fatalError("the fox must fit entirely inside the white tile's central area")
}
minX -= 2; minY -= 2; maxX += 2; maxY += 2

func bitmap(_ width: Int, _ height: Int) -> NSBitmapImageRep {
	let rep = NSBitmapImageRep(
		bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8,
		samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB,
		bytesPerRow: 0, bitsPerPixel: 0)!
	rep.size = NSSize(width: width, height: height)
	return rep
}

let mask = bitmap(maxX - minX + 1, maxY - minY + 1)
let maskPixels = mask.bitmapData!
for y in 0..<mask.pixelsHigh {
	for x in 0..<mask.pixelsWide {
		let i = y * mask.bytesPerRow + x * 4
		// Keep edge coverage while removing near-white paper and black texture.
		let alpha = max(0, min(1, (0.95 - light(minX + x, minY + y)) / 0.85))
		maskPixels[i] = 0; maskPixels[i+1] = 0; maskPixels[i+2] = 0
		maskPixels[i+3] = UInt8((alpha * 255).rounded())
	}
}
let mark = NSImage(cgImage: mask.cgImage!, size: mask.size)

func renderDock() throws {
	let size = 1024
	let rep = bitmap(size, size)
	NSGraphicsContext.saveGraphicsState()
	defer { NSGraphicsContext.restoreGraphicsState() }
	NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
	NSGraphicsContext.current!.imageInterpolation = .high
	NSColor(srgbRed: 247.0/255, green: 246.0/255, blue: 242.0/255, alpha: 1).setFill()
	NSRect(x: 0, y: 0, width: size, height: size).fill()
	let scale = 700.0 / max(mark.size.width, mark.size.height)
	let width = mark.size.width * scale, height = mark.size.height * scale
	mark.draw(in: NSRect(x: (Double(size) - width)/2, y: (Double(size) - height)/2, width: width, height: height))
	try rep.representation(using: .png, properties: [:])!
		.write(to: root.appendingPathComponent("resources/icon.png"), options: .atomic)
}

try renderDock()
