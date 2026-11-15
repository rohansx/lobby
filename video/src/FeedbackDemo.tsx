import { Series } from "remotion"
import { End, Title } from "./Cards"
import { Shot, VoiceOverlay } from "./Shot"

// Real screenshots from app.paperhq.ai and trylobby.app (see public/shots).
const scenes = [
  { frames: 90, el: <Title /> },
  { frames: 60, el: <Shot src="01-home.jpg" step="01" caption="Click what's broken" from={{ x: 784, y: 400, zoom: 1 }} to={{ x: 1400, y: 700, zoom: 1.6 }} /> },
  { frames: 90, el: <Shot src="02-picker.jpg" step="01" caption="Click what's broken" from={{ x: 1100, y: 600, zoom: 1.4 }} to={{ x: 722, y: 680, zoom: 2.3 }} /> },
  { frames: 120, el: <Shot src="03-widget.jpg" step="02" caption="A screenshot, the element and what led up to it" from={{ x: 800, y: 500, zoom: 1.2 }} to={{ x: 800, y: 470, zoom: 1.75 }} /> },
  {
    frames: 120,
    el: (
      <Shot src="03-widget.jpg" step="03" caption="Or just say it, in any Indian language or English" from={{ x: 800, y: 560, zoom: 1.75 }} to={{ x: 760, y: 630, zoom: 2.3 }}>
        <VoiceOverlay />
      </Shot>
    ),
  },
  { frames: 50, el: <Shot src="04-typed.jpg" step="04" caption="Send. Everything is attached" from={{ x: 800, y: 560, zoom: 1.9 }} to={{ x: 820, y: 600, zoom: 2.1 }} /> },
  { frames: 60, el: <Shot src="05-sent.jpg" step="04" caption="Send. Everything is attached" from={{ x: 800, y: 330, zoom: 2 }} to={{ x: 800, y: 330, zoom: 2.2 }} /> },
  { frames: 75, el: <Shot src="06-issues.jpg" step="05" caption="Claude groups reports into one issue" from={{ x: 784, y: 391, zoom: 1 }} to={{ x: 730, y: 280, zoom: 1.6 }} /> },
  { frames: 120, el: <Shot src="07-issue.jpg" step="05" caption="Claude groups reports into one issue" from={{ x: 860, y: 200, zoom: 1.35 }} to={{ x: 780, y: 230, zoom: 1.7 }} /> },
  { frames: 75, el: <Shot src="08-reports.jpg" step="05" caption="Every report, with the element outlined" from={{ x: 800, y: 420, zoom: 1.4 }} to={{ x: 700, y: 460, zoom: 1.8 }} /> },
  { frames: 120, el: <Shot src="09-packet.jpg" step="06" caption="Hand it to your coding agent" from={{ x: 880, y: 340, zoom: 1.6 }} to={{ x: 880, y: 600, zoom: 1.6 }} /> },
  { frames: 90, el: <End /> },
]

export const TOTAL = scenes.reduce((n, s) => n + s.frames, 0)

export const FeedbackDemo = () => (
  <Series>
    {scenes.map((s, i) => (
      <Series.Sequence key={i} durationInFrames={s.frames}>
        {s.el}
      </Series.Sequence>
    ))}
  </Series>
)
