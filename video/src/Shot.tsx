import { AbsoluteFill, Easing, Img, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig } from "remotion"
import type { ReactNode } from "react"
import { accent, bg, fontFamily, ink, line, muted } from "./theme"

// Screenshots are 1568x782; focus points below are in those pixels.
const SHOT_W = 1568
const SHOT_H = 782
const FRAME_W = 1680
const FRAME_H = Math.round((FRAME_W * SHOT_H) / SHOT_W)
const FRAME_TOP = 196

export interface Focus {
  x: number
  y: number
  zoom: number
}

interface ShotProps {
  src: string
  caption?: string
  step?: string
  from: Focus
  to: Focus
  children?: ReactNode // overlays drawn above the frame
}

// One screenshot in a window frame, with a slow push from one focus point to another.
export const Shot = ({ src, caption, step, from, to, children }: ShotProps) => {
  const frame = useCurrentFrame()
  const { durationInFrames, fps } = useVideoConfig()
  const t = interpolate(frame, [6, durationInFrames - 4], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.inOut(Easing.cubic) })
  const zoom = from.zoom + (to.zoom - from.zoom) * t
  const fx = from.x + (to.x - from.x) * t
  const fy = from.y + (to.y - from.y) * t
  // Keep the focus point centred in the frame while zoomed, without showing past the edges.
  const scale = (FRAME_W / SHOT_W) * zoom
  const left = clamp(FRAME_W / 2 - fx * scale, FRAME_W - SHOT_W * scale, 0)
  const top = clamp(FRAME_H / 2 - fy * scale, FRAME_H - SHOT_H * scale, 0)
  const enter = spring({ frame, fps, config: { damping: 200 }, durationInFrames: 14 })

  return (
    <AbsoluteFill style={{ background: bg, fontFamily, opacity: interpolate(frame, [0, 8], [0, 1], { extrapolateRight: "clamp" }) }}>
      {caption && <Caption text={caption} step={step} />}
      <div
        style={{
          position: "absolute",
          left: (1920 - FRAME_W) / 2,
          top: FRAME_TOP + (1 - enter) * 24,
          width: FRAME_W,
          height: FRAME_H,
          borderRadius: 18,
          overflow: "hidden",
          border: `1px solid ${line}`,
          boxShadow: "0 40px 90px -40px rgba(31,28,23,.45)",
        }}
      >
        <Img src={staticFile(`shots/${src}`)} style={{ position: "absolute", left, top, width: SHOT_W * scale, height: SHOT_H * scale }} />
      </div>
      {children}
    </AbsoluteFill>
  )
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v))

const Caption = ({ text, step }: { text: string; step?: string }) => {
  const frame = useCurrentFrame()
  const o = interpolate(frame, [4, 16], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })
  return (
    <div style={{ position: "absolute", top: 62, left: 120, right: 120, display: "flex", alignItems: "baseline", gap: 22, opacity: o, transform: `translateY(${(1 - o) * 10}px)` }}>
      {step && <span style={{ fontSize: 26, color: accent, fontWeight: 500, letterSpacing: "0.02em" }}>{step}</span>}
      <span style={{ fontSize: 58, color: ink, fontWeight: 400, letterSpacing: "-0.035em" }}>{text}</span>
    </div>
  )
}

// A recording pill with live-looking bars, drawn over the Speak scene.
export const VoiceOverlay = () => {
  const frame = useCurrentFrame()
  const o = interpolate(frame, [10, 22], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" })
  const secs = Math.min(9, Math.floor(frame / 30))
  return (
    <div
      style={{
        position: "absolute",
        left: "50%",
        bottom: 70,
        transform: `translateX(-50%) scale(${0.96 + o * 0.04})`,
        opacity: o,
        display: "flex",
        alignItems: "center",
        gap: 22,
        padding: "20px 34px",
        borderRadius: 999,
        background: ink,
        color: bg,
        fontFamily,
        boxShadow: "0 24px 60px -24px rgba(31,28,23,.6)",
      }}
    >
      <span style={{ width: 16, height: 16, borderRadius: 999, background: "#ef4444", opacity: 0.6 + 0.4 * Math.sin(frame / 5) }} />
      <div style={{ display: "flex", alignItems: "center", gap: 6, height: 48 }}>
        {Array.from({ length: 28 }, (_, i) => {
          const h = 10 + Math.abs(Math.sin(frame / 4 + i * 0.7) * Math.cos(frame / 9 + i * 0.3)) * 38
          return <span key={i} style={{ width: 6, height: h, borderRadius: 3, background: i % 5 === 0 ? accent : "#d8d2c6" }} />
        })}
      </div>
      <span style={{ fontSize: 26, fontVariantNumeric: "tabular-nums", color: "#d8d2c6" }}>0:0{secs}</span>
      <span style={{ fontSize: 24, color: muted }}>हिंदी · தமிழ் · English</span>
    </div>
  )
}
