# Video Match Spec (YouTube Short: w8v8M1cPgEc)

## Purpose
This document locks visual and gameplay direction to the reference Short:
`https://youtube.com/shorts/w8v8M1cPgEc`

Scope: The in-game frame content is authoritative. Social-video overlays and edit artifacts are not.

## Verified Clip Metadata
- Video ID: `w8v8M1cPgEc`
- Title: `this is why i love rocket league 😍`
- Duration: `~21s`
- Gameplay format: vertical crop of standard Rocket League-style 3D gameplay
- Observed cadence: high frame rate capture with smooth motion (target 60fps+)

## Visual Direction Lock

### Arena
- Enclosed indoor stadium.
- Hex-panel transparent side/ceiling walls.
- Bright overhead strip lighting arrays.
- Team-side color identity:
  - Orange half: warm key accents and banners.
  - Blue half: cool key accents and backboard glow.
- High-contrast pitch lines and readable field segmentation.

### Materials and Surfaces
- Ball: brushed-metal + composite panels with subtle emissive details.
- Car paint: high-gloss metallic finish with clear highlight streaks.
- Turf: dark green, low-noise spec response for strong ball readability.
- Glass/wall: translucent with crisp edge glow, no heavy opacity haze.

### Lighting and Post Process
- Stadium key lights dominate scene readability.
- Moderate bloom on bright fixtures and boost exhaust.
- Mild motion blur only; keep object readability while boosting.
- Color grade:
  - Slight teal/cool global tone.
  - Warm highlights on orange-side impacts/exhaust.
- No filmic grain in competitive preset.

### Camera and HUD
- Tight chase camera behind player car with stable framing of ball + goal path.
- HUD layout:
  - Top-center score + match timer (orange vs blue).
  - Event text feedback (e.g., "SHOT ON GOAL").
- Gameplay-first clarity: UI remains legible during aerial and wall play.

### VFX
- Boost trail with bright emissive core and short-lived particle tail.
- Contact sparks on hard touches and wall transitions.
- Goal/shot feedback effects are visible but not screen-filling.

## Gameplay Behavior Lock (From Clip)
- Core mode behavior is competitive soccer (no chaos mutators shown).
- Possession chain behavior emphasized:
  - Ground dribble control.
  - Wall carry.
  - Air dribble sequence.
  - Last-second attacking sequence near `0:00`.
- Ball remains active during final moments until play resolves (zero-second continuation behavior).
- High mechanical precision in car-ball control is expected.

## Explicit Non-Goals
- Do not copy creator text overlays from social edit (e.g., "This is why I love Rocket League").
- Do not include blurred top/bottom bars from vertical social crop in game output.
- Do not copy trademarks/branding assets directly; create original but equivalent-quality visual language.

## UE 5.7 Implementation Targets
- Renderer baseline:
  - Nanite for stadium geometry.
  - Lumen for GI/reflections on quality tiers.
  - Virtual Shadow Maps for crisp contact shadows.
  - TSR default; DLSS/FSR/XeSS adapters by platform.
- Competitive visual preset:
  - Disable heavy depth-of-field.
  - Reduce expensive translucency layers.
  - Keep boost and ball readability above cinematic effects.
- Performance goals tied to this style:
  - Competitive: 120fps console performance mode, 240fps capable on high-end PC.
  - Quality: stable 60fps with enhanced reflections and GI.

## Acceptance Criteria (Visual)
- Screenshot and replay comparisons from 6 canonical moments:
  1. Defensive blue goal box sequence.
  2. Midfield dribble acceleration.
  3. Wall carry transition.
  4. Aerial control with ceiling wall context.
  5. `0:00` attacking continuation.
  6. Shot-on-goal feedback readability.
- For each canonical moment, art review must sign off:
  - silhouette clarity,
  - ball readability,
  - lighting consistency,
  - HUD legibility.

## Acceptance Criteria (Gameplay Feel)
- 1v1 possession drills must support controlled dribble and wall-to-air transition with low input lag.
- Hit response and bounce behavior must be predictable under 30ms, 60ms, and 90ms simulated latency buckets.
- End-of-clock behavior at `0:00` must preserve continuity while ball remains in play.
