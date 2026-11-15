import { AbsoluteFill, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion"
import { bg, fontFamily, ink, muted } from "./theme"

const useEnter = () => {
  const frame = useCurrentFrame()
  const { fps, durationInFrames } = useVideoConfig()
  const s = spring({ frame, fps, config: { damping: 200 }, durationInFrames: 24 })
  const out = interpolate(frame, [durationInFrames - 8, durationInFrames], [1, 0], { extrapolateLeft: "clamp" })
  return { s, out }
}

export const Title = () => {
  const { s, out } = useEnter()
  return (
    <AbsoluteFill style={{ background: bg, fontFamily, justifyContent: "center", paddingLeft: 160, opacity: out }}>
      <div style={{ fontSize: 30, color: muted, marginBottom: 26, opacity: s }}>Lobby Feedback</div>
      <div style={{ fontSize: 112, lineHeight: 1.02, letterSpacing: "-0.05em", fontWeight: 400, color: ink, transform: `translateY(${(1 - s) * 30}px)`, opacity: s }}>
        Your first users find the bugs.
        <br />
        <span style={{ color: "#b1aa9c" }}>Lobby brings them to you.</span>
      </div>
    </AbsoluteFill>
  )
}

export const End = () => {
  const { s } = useEnter()
  return (
    <AbsoluteFill style={{ background: bg, fontFamily, justifyContent: "center", alignItems: "center", gap: 34 }}>
      <Img src={staticFile("logo.svg")} style={{ width: 96, height: 96, opacity: s, transform: `scale(${0.9 + s * 0.1})` }} />
      <div style={{ fontSize: 84, letterSpacing: "-0.045em", color: ink, opacity: s }}>
        Lobby <span style={{ color: "#b1aa9c" }}>· trylobby.app</span>
      </div>
      <div style={{ fontSize: 32, color: muted, opacity: s }}>One script tag. Free to start, or self-host.</div>
    </AbsoluteFill>
  )
}
