import { Composition } from "remotion"
import { FeedbackDemo, TOTAL } from "./FeedbackDemo"

export const Root = () => (
  <Composition id="FeedbackDemo" component={FeedbackDemo} durationInFrames={TOTAL} fps={30} width={1920} height={1080} />
)
