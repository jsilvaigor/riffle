import SwiftUI

/// Riffle's timeline (`design/components/Scrubber.md`): drag to seek, with the
/// trickplay frame for the finger's time drawn above the thumb from memory.
struct Scrubber: View {
    var duration: Double
    var time: Double
    var buffered: Double
    var isScrubbing: Bool
    var scrubLabel: String?
    var previewAspect: Double
    var preview: (Double) -> CGImage?
    var onBegin: (Double) -> Void
    var onChange: (Double, PlayerModel.ScrubSpeed) -> Void
    var onEnd: () -> Void
    var onStep: (Double) -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var dragAnchorX: CGFloat?
    @State private var dragTime: Double = 0

    var body: some View {
        HStack(spacing: Riffle.Spacing.s2 + 4) {
            Text(Format.time(time))
                .font(Riffle.Typography.timecode)
                .monospacedDigit()
                .foregroundStyle(Riffle.Palette.ink)
            GeometryReader { geo in
                track(width: geo.size.width)
                    .frame(height: Riffle.minimumHitTarget)
                    .frame(maxHeight: .infinity, alignment: .center)
                    .contentShape(.rect)
                    .gesture(drag(width: geo.size.width))
                    .overlay(alignment: .topLeading) {
                        if isScrubbing { previewFrame(trackWidth: geo.size.width) }
                    }
            }
            .frame(height: Riffle.minimumHitTarget)
            Text(Format.remaining(max(duration - time, 0)))
                .font(Riffle.Typography.timecode)
                .monospacedDigit()
                .foregroundStyle(Riffle.Palette.ink)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Playback position")
        .accessibilityValue("\(Format.time(time)) of \(Format.time(duration))")
        .accessibilityAdjustableAction { direction in
            onStep(direction == .increment ? 10 : -10)
        }
    }

    private var fraction: CGFloat { duration > 0 ? CGFloat(min(max(time / duration, 0), 1)) : 0 }
    private var bufferedFraction: CGFloat { duration > 0 ? CGFloat(min(max(buffered / duration, 0), 1)) : 0 }

    private func track(width: CGFloat) -> some View {
        let height: CGFloat = isScrubbing ? 8 : Riffle.scrubberTrackHeight
        return ZStack(alignment: .leading) {
            Capsule().fill(Riffle.Palette.line)
            Capsule().fill(Riffle.Palette.inkMuted).frame(width: width * max(bufferedFraction, fraction))
            Capsule().fill(Riffle.Palette.citron).frame(width: width * fraction)
        }
        .frame(height: height)
        .overlay(alignment: .leading) {
            Circle()
                .fill(Riffle.Palette.citron)
                .frame(width: 14, height: 14)
                .scaleEffect(isScrubbing && !reduceMotion ? 1.6 : 1)
                .offset(x: width * fraction - 7)
        }
        .animation(.easeOut(duration: 0.15), value: isScrubbing)
    }

    /// Long side 160 pt, shaped like the video; time under it on `scrim`.
    @ViewBuilder private func previewFrame(trackWidth: CGFloat) -> some View {
        let size = previewAspect >= 1
            ? CGSize(width: 160, height: 160 / previewAspect)
            : CGSize(width: 160 * previewAspect, height: 160)
        let labelHeight: CGFloat = 24
        let x = min(max(trackWidth * fraction - size.width / 2, -40), trackWidth - size.width + 40)
        VStack(spacing: Riffle.Spacing.s1) {
            if let label = scrubLabel {
                Text(label)
                    .font(Riffle.Typography.caption)
                    .foregroundStyle(Riffle.Palette.ink)
                    .fixedSize()
            }
            ZStack {
                Rectangle().fill(Riffle.Palette.stage)
                if let img = preview(time) {
                    Image(decorative: img, scale: 1).resizable().interpolation(.medium).scaledToFill()
                } else {
                    Text("Preview not ready")
                        .font(Riffle.Typography.caption)
                        .foregroundStyle(Riffle.Palette.ink)
                }
            }
            .frame(width: size.width, height: size.height)
            .clipShape(.rect(cornerRadius: Riffle.Radius.md))
            .overlay(RoundedRectangle(cornerRadius: Riffle.Radius.md).strokeBorder(Riffle.Palette.ink, lineWidth: 2))
            Text(Format.time(time))
                .font(Riffle.Typography.timecode)
                .monospacedDigit()
                .foregroundStyle(Riffle.Palette.ink)
                .padding(.horizontal, Riffle.Spacing.s2)
                .frame(height: labelHeight)
                .background(Riffle.Palette.scrim, in: .capsule)
        }
        .frame(width: size.width)
        .fixedSize()
        .alignmentGuide(.top) { d in d.height + 4 }
        .offset(x: x)
        .allowsHitTesting(false)
    }

    private func drag(width: CGFloat) -> some Gesture {
        DragGesture(minimumDistance: 0, coordinateSpace: .local)
            .onChanged { value in
                guard width > 0, duration > 0 else { return }
                // Height above the track's centre line picks the speed.
                let above = max(0, Riffle.minimumHitTarget / 2 - value.location.y)
                let speed = PlayerModel.ScrubSpeed(heightAboveTrack: above)
                if let anchor = dragAnchorX {
                    let dx = value.location.x - anchor
                    dragTime += Double(dx / width) * duration * speed.factor
                    dragTime = min(max(dragTime, 0), duration)
                    dragAnchorX = value.location.x
                    onChange(dragTime, speed)
                } else {
                    // Touching the track jumps there, then the drag is relative.
                    dragAnchorX = value.location.x
                    dragTime = Double(min(max(value.location.x / width, 0), 1)) * duration
                    onBegin(dragTime)
                    onChange(dragTime, speed)
                }
            }
            .onEnded { _ in
                dragAnchorX = nil
                onEnd()
            }
    }
}
