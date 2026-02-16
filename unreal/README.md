# Unreal 5.7 Integration Starter

This folder contains configuration templates aligned with:
- `/Users/waqarazim/Desktop/rocketleague/PROJECT_VELOCITY_PRD_ARCHITECTURE_AGENT_PLAYBOOK.md`
- `/Users/waqarazim/Desktop/rocketleague/docs/graphics/VIDEO_MATCH_SPEC.md`

## Templates
- `templates/Config/DefaultEngine.ini`
- `templates/Config/DefaultGame.ini`
- `templates/Config/Scalability.ini`

## Usage
1. Create a UE 5.7 C++ project named `Velocity`.
2. Copy these files into `<VelocityProject>/Config/`.
3. Enable plugins:
   - OnlineSubsystemEOS
   - ReplicationGraph
   - ChaosVehicles
   - EnhancedInput
4. Build Dedicated Server target and validate 120Hz server tick profile.

## Competitive Preset Goals
- Prioritize frame-time stability and input latency over cinematic effects.
- Keep ball/car readability high under boost and aerial play.
- Verify scoreboard/HUD clarity on 1080p and 1440p in motion.
