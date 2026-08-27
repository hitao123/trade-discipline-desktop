import AppKit
import Foundation
import Vision

struct OCRLine: Encodable {
    let text: String
    let confidence: Float
    let minX: CGFloat
    let maxY: CGFloat

    enum CodingKeys: String, CodingKey {
        case text
        case confidence
    }
}

struct OCRResult: Encodable {
    let lines: [OCRLine]
}

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(Data((message + "\n").utf8))
    exit(1)
}

guard CommandLine.arguments.count == 2 else {
    fail("usage: plain-rule-ocr <image-path>")
}

let imagePath = CommandLine.arguments[1]
guard let image = NSImage(contentsOfFile: imagePath) else {
    fail("无法读取所选图片")
}
var proposedRect = CGRect(origin: .zero, size: image.size)
guard let cgImage = image.cgImage(forProposedRect: &proposedRect, context: nil, hints: nil) else {
    fail("无法解析所选图片")
}

let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate
request.recognitionLanguages = ["zh-Hans", "en-US"]
request.usesLanguageCorrection = true

do {
    try VNImageRequestHandler(cgImage: cgImage).perform([request])
    let lines = (request.results ?? []).compactMap { observation -> OCRLine? in
        guard let candidate = observation.topCandidates(1).first else { return nil }
        return OCRLine(
            text: candidate.string,
            confidence: candidate.confidence,
            minX: observation.boundingBox.minX,
            maxY: observation.boundingBox.maxY
        )
    }.sorted { left, right in
        if abs(left.maxY - right.maxY) > 0.015 { return left.maxY > right.maxY }
        return left.minX < right.minX
    }
    let data = try JSONEncoder().encode(OCRResult(lines: lines))
    FileHandle.standardOutput.write(data)
} catch {
    fail("本地文字识别失败：\(error.localizedDescription)")
}
