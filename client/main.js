import * as THREE from "https://cdn.jsdelivr.net/npm/three@0.167.1/build/three.module.js";

const SIM_SCALE = 0.01;
const HUD_EVENT_TIMEOUT_MS = 1200;
const INPUT_SEND_HZ = 60;
const OFFLINE_TICK_HZ = 120;
const ARENA_LENGTH_UU = 8192;
const ARENA_WIDTH_UU = 10240;
const ARENA_HEIGHT_UU = 2044;
const GOAL_WIDTH_UU = 1785.51;
const GOAL_HEIGHT_UU = 642.775;
const CAR_RADIUS_UU = 95;
const BALL_RADIUS_UU = 91.25;
const MAX_CAR_SPEED = 2300;
const MAX_DRIVE_SPEED = 1410;
const THROTTLE_ACCEL = 1600;
const BRAKE_ACCEL = 3500;
const BOOST_ACCEL = 991.666;
const TURN_RATE = 3.4;
const GRAVITY = -650;
const JUMP_VELOCITY = 292;
const JUMP_HOLD_ACCEL = 1460;
const JUMP_HOLD_MAX = 0.2;
const STICKY_FORCE = 325;
const STICKY_TIME = 3 / 120;
const DOUBLE_JUMP_MAX = 1.25;
const BALL_RESTITUTION = 0.6;
const WALL_RESTITUTION = 0.78;
const CAR_BALL_ELASTICITY = 0.94;
const GROUND_FRICTION = 0.9965;
const COAST_FRICTION = 0.996;
const LATERAL_GRIP = 0.78;
const HANDBRAKE_GRIP = 0.9;
const HANDBRAKE_TURN_BOOST = 1.35;
const AIR_RESISTANCE = 0.9992;
const AIR_THROTTLE_ACCEL = 66.667;
const AIR_REVERSE_ACCEL = 33.334;
const BALL_MAX_SPEED = 6000;

const canvas = document.getElementById("game-canvas");
const menu = document.getElementById("menu");
const startOnlineBtn = document.getElementById("start-online-btn");
const startOfflineBtn = document.getElementById("start-offline-btn");
const gatewayInput = document.getElementById("gateway-url");
const displayNameInput = document.getElementById("display-name");

const statusEl = document.getElementById("status");
const eventEl = document.getElementById("match-event");
const timerEl = document.getElementById("timer");
const scoreOrangeEl = document.getElementById("score-orange");
const scoreBlueEl = document.getElementById("score-blue");
const boostEl = document.getElementById("boost");
const pingEl = document.getElementById("ping");
const playersEl = document.getElementById("players");

const state = {
  mode: "idle",
  gatewayURL: "http://localhost:9000",
  token: "",
  playerID: "",
  displayName: "Pilot",
  ticketID: "",
  ws: null,
  matchID: "",
  localCarID: "",
  pingSentAt: 0,
  pingMS: null,
  seq: 1,
  connected: false,
  cars: new Map(),
  localCarState: null,
  ballVisual: null,
  lastEventSig: "",
  offline: {
    active: false,
    accumulator: 0,
    matchState: null,
    ctx: {},
  },
};

const keys = new Set();

const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, powerPreference: "high-performance" });
renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.toneMapping = THREE.ACESFilmicToneMapping;
renderer.toneMappingExposure = 1.03;
renderer.outputColorSpace = THREE.SRGBColorSpace;
renderer.shadowMap.enabled = true;
renderer.shadowMap.type = THREE.PCFSoftShadowMap;

const scene = new THREE.Scene();
scene.background = new THREE.Color(0x061121);
scene.fog = new THREE.Fog(0x061121, 20, 130);

const camera = new THREE.PerspectiveCamera(65, window.innerWidth / window.innerHeight, 0.1, 200);
camera.position.set(-14, 9, 14);

initEnvironment();
startRenderLoop();
bindInputHandlers();
startNetworkLoops();

startOnlineBtn.addEventListener("click", async () => {
  await startOnlineMatchFlow();
});

startOfflineBtn.addEventListener("click", () => {
  startOfflineMatchFlow();
});

function initEnvironment() {
  const hemi = new THREE.HemisphereLight(0xa2cdfa, 0x0f1317, 0.65);
  scene.add(hemi);

  const keyLight = new THREE.DirectionalLight(0xf3f8ff, 1.2);
  keyLight.position.set(-22, 28, -7);
  keyLight.castShadow = true;
  keyLight.shadow.mapSize.set(2048, 2048);
  keyLight.shadow.camera.left = -65;
  keyLight.shadow.camera.right = 65;
  keyLight.shadow.camera.top = 65;
  keyLight.shadow.camera.bottom = -65;
  scene.add(keyLight);

  const rim1 = new THREE.PointLight(0x53a9ff, 24, 110, 2.2);
  rim1.position.set(44, 20, 0);
  scene.add(rim1);

  const rim2 = new THREE.PointLight(0xff9853, 24, 110, 2.2);
  rim2.position.set(-44, 20, 0);
  scene.add(rim2);

  const pitchGeo = new THREE.PlaneGeometry(ARENA_LENGTH_UU * SIM_SCALE, ARENA_WIDTH_UU * SIM_SCALE, 1, 1);
  const pitchMat = new THREE.MeshStandardMaterial({
    color: 0x0a2f29,
    roughness: 0.87,
    metalness: 0.06,
  });
  const pitch = new THREE.Mesh(pitchGeo, pitchMat);
  pitch.rotation.x = -Math.PI / 2;
  pitch.receiveShadow = true;
  scene.add(pitch);

  const centerLine = new THREE.Mesh(
    new THREE.BoxGeometry(0.2, 0.02, ARENA_WIDTH_UU * SIM_SCALE),
    new THREE.MeshBasicMaterial({ color: 0x7ac9ff })
  );
  centerLine.position.y = 0.02;
  scene.add(centerLine);

  const centerCircle = new THREE.Mesh(
    new THREE.RingGeometry(4.4, 4.7, 64),
    new THREE.MeshBasicMaterial({ color: 0x8ed8ff, side: THREE.DoubleSide })
  );
  centerCircle.rotation.x = -Math.PI / 2;
  centerCircle.position.y = 0.03;
  scene.add(centerCircle);

  const arenaBox = new THREE.BoxGeometry(
    ARENA_LENGTH_UU * SIM_SCALE,
    ARENA_HEIGHT_UU * SIM_SCALE,
    ARENA_WIDTH_UU * SIM_SCALE
  );
  const arenaEdges = new THREE.EdgesGeometry(arenaBox);
  const arenaWire = new THREE.LineSegments(
    arenaEdges,
    new THREE.LineBasicMaterial({ color: 0x74b9ff, transparent: true, opacity: 0.5 })
  );
  arenaWire.position.y = (ARENA_HEIGHT_UU * SIM_SCALE) / 2;
  scene.add(arenaWire);

  addGoalFrame("orange", -(ARENA_LENGTH_UU * SIM_SCALE) / 2, 0xf67e2e);
  addGoalFrame("blue", (ARENA_LENGTH_UU * SIM_SCALE) / 2, 0x3b8efc);

  const ball = new THREE.Mesh(
    new THREE.SphereGeometry(ballRadiusVisual(), 48, 48),
    new THREE.MeshPhysicalMaterial({
      color: 0xbcc5cf,
      metalness: 0.72,
      roughness: 0.32,
      clearcoat: 0.4,
      clearcoatRoughness: 0.3,
    })
  );
  ball.castShadow = true;
  ball.receiveShadow = true;
  ball.position.set(0, ballRadiusVisual(), 0);
  scene.add(ball);
  state.ballVisual = ball;

  addStadiumLights();
}

function addStadiumLights() {
  for (let i = -5; i <= 5; i += 1) {
    const bar = new THREE.Mesh(
      new THREE.BoxGeometry(7, 0.11, 0.25),
      new THREE.MeshBasicMaterial({ color: 0xecf6ff })
    );
    bar.position.set(i * 8.8, 15.7, 32.5);
    scene.add(bar);

    const barMirror = bar.clone();
    barMirror.position.z = -32.5;
    scene.add(barMirror);
  }
}

function addGoalFrame(team, x, color) {
  const postMaterial = new THREE.MeshStandardMaterial({
    color,
    metalness: 0.72,
    roughness: 0.36,
    emissive: new THREE.Color(color).multiplyScalar(0.11),
  });

  const frame = new THREE.Group();

  const goalHalf = (GOAL_WIDTH_UU * SIM_SCALE) / 2;
  const goalHeight = GOAL_HEIGHT_UU * SIM_SCALE;
  const postL = new THREE.Mesh(new THREE.CylinderGeometry(0.2, 0.2, goalHeight, 16), postMaterial);
  postL.position.set(x, goalHeight / 2, -goalHalf);
  frame.add(postL);

  const postR = postL.clone();
  postR.position.z = goalHalf;
  frame.add(postR);

  const cross = new THREE.Mesh(new THREE.CylinderGeometry(0.2, 0.2, goalHalf * 2, 16), postMaterial);
  cross.rotation.x = Math.PI / 2;
  cross.position.set(x, goalHeight, 0);
  frame.add(cross);

  const backPlate = new THREE.Mesh(
    new THREE.PlaneGeometry(4.5, goalHeight),
    new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.14, side: THREE.DoubleSide })
  );
  backPlate.position.set(team === "orange" ? x - 2.0 : x + 2.0, goalHeight / 2, 0);
  backPlate.rotation.y = team === "orange" ? Math.PI / 2 : -Math.PI / 2;
  frame.add(backPlate);

  scene.add(frame);
}

function createCarVisual(carState) {
  const group = new THREE.Group();

  const baseColor = carState.is_bot
    ? 0x9b9b9b
    : carState.team === "orange"
      ? 0xf8a44a
      : 0x5ea5ff;
  const accentColor = carState.team === "orange" ? 0xff7d2c : 0x4f86ff;

  const bodyMat = new THREE.MeshPhysicalMaterial({
    color: baseColor,
    metalness: 0.78,
    roughness: 0.24,
    clearcoat: 0.62,
    clearcoatRoughness: 0.16,
    emissive: new THREE.Color(accentColor).multiplyScalar(0.08),
  });

  const body = new THREE.Mesh(new THREE.BoxGeometry(2.15, 0.75, 1.2), bodyMat);
  body.castShadow = true;
  body.receiveShadow = true;
  body.position.y = -0.25;
  group.add(body);

  const roof = new THREE.Mesh(
    new THREE.BoxGeometry(1.28, 0.42, 0.95),
    new THREE.MeshPhysicalMaterial({
      color: 0xfaf8f0,
      metalness: 0.25,
      roughness: 0.13,
      transmission: 0.2,
      transparent: true,
      opacity: 0.93,
    })
  );
  roof.position.y = 0.28;
  roof.castShadow = true;
  group.add(roof);

  const spoiler = new THREE.Mesh(new THREE.BoxGeometry(0.28, 0.1, 1.1), bodyMat);
  spoiler.position.set(-1.08, -0.02, 0);
  group.add(spoiler);

  const wheelMat = new THREE.MeshStandardMaterial({ color: 0x111317, metalness: 0.35, roughness: 0.55 });
  const wheelGeo = new THREE.CylinderGeometry(0.28, 0.28, 0.2, 18);
  const wheelOffsets = [
    [0.72, -0.67, 0.56],
    [0.72, -0.67, -0.56],
    [-0.72, -0.67, 0.56],
    [-0.72, -0.67, -0.56],
  ];
  for (const [x, y, z] of wheelOffsets) {
    const wheel = new THREE.Mesh(wheelGeo, wheelMat);
    wheel.rotation.z = Math.PI / 2;
    wheel.position.set(x, y, z);
    group.add(wheel);
  }

  const boost = new THREE.PointLight(accentColor, 2.8, 8, 2.4);
  boost.position.set(-1.15, -0.2, 0);
  group.add(boost);

  const tag = createNameTag(carState.display_name || carState.player_id, carState.is_bot);
  tag.position.y = 1.28;
  group.add(tag);

  group.userData = {
    boostLight: boost,
    targetPos: new THREE.Vector3(),
    targetRotY: 0,
    tag,
  };

  scene.add(group);
  return group;
}

function createNameTag(name, isBot) {
  const labelCanvas = document.createElement("canvas");
  labelCanvas.width = 256;
  labelCanvas.height = 64;
  const ctx = labelCanvas.getContext("2d");

  ctx.clearRect(0, 0, labelCanvas.width, labelCanvas.height);
  ctx.fillStyle = isBot ? "rgba(255, 170, 120, 0.82)" : "rgba(165, 220, 255, 0.82)";
  ctx.fillRect(2, 2, labelCanvas.width - 4, labelCanvas.height - 4);
  ctx.fillStyle = "rgba(8, 18, 34, 0.95)";
  ctx.font = "700 30px Rajdhani, sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(name.slice(0, 18), labelCanvas.width / 2, labelCanvas.height / 2 + 2);

  const tex = new THREE.CanvasTexture(labelCanvas);
  tex.needsUpdate = true;
  const mat = new THREE.SpriteMaterial({ map: tex, transparent: true });
  const sprite = new THREE.Sprite(mat);
  sprite.scale.set(3.8, 1.1, 1);
  return sprite;
}

function startRenderLoop() {
  let last = performance.now();
  const loop = (now) => {
    const dt = Math.min((now - last) / 1000, 0.1);
    last = now;

    stepOfflineSimulation(dt);
    updateVisuals(dt);
    renderer.render(scene, camera);
    requestAnimationFrame(loop);
  };
  requestAnimationFrame(loop);
}

function updateVisuals(dt) {
  for (const [carID, carVisual] of state.cars) {
    const isLocal = carID === state.localCarID;
    const posLerp = isLocal ? 0.44 : 0.24;
    const rotLerp = isLocal ? 0.42 : 0.24;
    carVisual.position.lerp(carVisual.userData.targetPos, posLerp);
    carVisual.rotation.y = lerpAngle(carVisual.rotation.y, carVisual.userData.targetRotY, rotLerp);

    const localSpeed = Math.hypot(carVisual.userData.velX || 0, carVisual.userData.velY || 0);
    const boostIntensity = Math.min(Math.max(localSpeed / 2750, 0.2), 1.0);
    carVisual.userData.boostLight.intensity = 1.0 + boostIntensity * 2.8;
  }

  if (state.ballVisual?.userData?.targetPos) {
    state.ballVisual.position.lerp(state.ballVisual.userData.targetPos, 0.35);
  }

  const local = state.localCarState;
  if (local) {
    const localVisual = state.cars.get(state.localCarID);
    const yaw = (local.rotation.yaw * Math.PI) / 180;
    const carPos = localVisual ? localVisual.position : toScenePos(local.position);
    const speed = Math.hypot(local.velocity.x || 0, local.velocity.y || 0);

    const followDistance = 8.4 + Math.min(speed / 2300, 1) * 2.2;
    const height = 4.3;
    const desiredCam = new THREE.Vector3(
      carPos.x - Math.cos(yaw) * followDistance,
      carPos.y + height,
      carPos.z - Math.sin(yaw) * followDistance
    );

    camera.position.lerp(desiredCam, 0.11);

    const lookAhead = new THREE.Vector3(
      carPos.x + Math.cos(yaw) * 4.4,
      carPos.y + 1.2,
      carPos.z + Math.sin(yaw) * 4.4
    );
    if (state.ballVisual) {
      lookAhead.lerp(state.ballVisual.position, 0.28);
    }
    camera.lookAt(lookAhead);
  }
}

function bindInputHandlers() {
  window.addEventListener("keydown", (e) => {
    if (menu.style.display !== "none" && document.activeElement && document.activeElement.tagName === "INPUT") {
      return;
    }
    keys.add(e.code);
    if (isControlKey(e.code)) {
      e.preventDefault();
    }
  });

  window.addEventListener("keyup", (e) => {
    keys.delete(e.code);
    if (isControlKey(e.code)) {
      e.preventDefault();
    }
  });

  window.addEventListener("resize", () => {
    renderer.setSize(window.innerWidth, window.innerHeight);
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
  });

  window.addEventListener("blur", () => {
    keys.clear();
  });
}

function isControlKey(code) {
  return (
    code === "KeyW" ||
    code === "KeyA" ||
    code === "KeyS" ||
    code === "KeyD" ||
    code === "ArrowUp" ||
    code === "ArrowDown" ||
    code === "ArrowLeft" ||
    code === "ArrowRight" ||
    code === "ShiftLeft" ||
    code === "ShiftRight" ||
    code === "Space" ||
    code === "ControlLeft" ||
    code === "ControlRight"
  );
}

function startNetworkLoops() {
  setInterval(() => {
    if (state.mode !== "online" || !state.ws || state.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    const input = buildInputPayload();
    const envelope = {
      type: "input",
      input,
    };
    state.ws.send(JSON.stringify(envelope));
  }, Math.round(1000 / INPUT_SEND_HZ));

  setInterval(() => {
    if (state.mode !== "online" || !state.ws || state.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    state.pingSentAt = performance.now();
    state.ws.send(JSON.stringify({ type: "ping" }));
  }, 2000);
}

function readControlState() {
  const throttle = (keys.has("KeyW") || keys.has("ArrowUp") ? 1 : 0) + (keys.has("KeyS") || keys.has("ArrowDown") ? -1 : 0);
  const steer = (keys.has("KeyD") || keys.has("ArrowRight") ? 1 : 0) + (keys.has("KeyA") || keys.has("ArrowLeft") ? -1 : 0);
  return {
    throttle: clamp(throttle, -1, 1),
    steer: clamp(steer, -1, 1),
    boost: keys.has("ShiftLeft") || keys.has("ShiftRight"),
    jump: keys.has("Space"),
    handbrake: keys.has("ControlLeft") || keys.has("ControlRight"),
  };
}

function buildInputPayload() {
  const control = readControlState();

  const payload = {
    player_id: state.playerID,
    sequence: state.seq++,
    throttle: control.throttle,
    steer: control.steer,
    boost: control.boost,
    jump: control.jump,
    handbrake: control.handbrake,
    client_ms: Date.now(),
  };
  return payload;
}

async function startOnlineMatchFlow() {
  if (startOnlineBtn.disabled || startOfflineBtn.disabled) {
    return;
  }

  try {
    setStartButtonsBusy(true);
    stopOfflineMode();
    closeOnlineSocket();
    clearLiveState();
    state.mode = "online";
    state.gatewayURL = gatewayInput.value.trim() || "http://localhost:9000";
    state.displayName = (displayNameInput.value.trim() || "Pilot").slice(0, 24);

    setStatus("Authenticating...");
    const auth = await requestJSON(`${state.gatewayURL}/v1/auth/guest`, {
      method: "POST",
      body: JSON.stringify({ display_name: state.displayName }),
    });

    state.token = auth.token;
    state.playerID = auth.player_id;
    state.localCarID = auth.player_id;

    setStatus("Searching for match...");
    const join = await requestJSON(`${state.gatewayURL}/v1/matchmaking/join`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({ region: "us-east", playlist: "ranked-1v1", mmr: 1200 }),
    });
    state.ticketID = join.ticket_id;

    const assignment = await pollForAssignment(state.ticketID);
    state.matchID = assignment.match_id;

    setStatus(assignment.bot_fill ? "Match found (bot fill)" : "Match found");
    menu.style.display = "none";
    if (document.activeElement) {
      document.activeElement.blur();
    }
    window.focus();
    canvas.focus();

    await connectWebSocket(assignment.server_addr);
  } catch (err) {
    console.error(err);
    setStatus(`Online unavailable (${err.message || "failed"}). Starting offline...`);
    startOfflineMatchFlow(true);
  } finally {
    setStartButtonsBusy(false);
  }
}

function startOfflineMatchFlow(fromFallback = false) {
  state.mode = "offline";
  closeOnlineSocket();
  stopOfflineMode();
  clearLiveState();
  state.displayName = (displayNameInput.value.trim() || "Pilot").slice(0, 24);
  state.playerID = state.playerID || `offline_${Date.now()}`;
  state.localCarID = state.playerID;
  state.offline = createOfflineSession(state.displayName, state.playerID);
  menu.style.display = "none";
  if (document.activeElement) {
    document.activeElement.blur();
  }
  window.focus();
  canvas.focus();
  setStatus(fromFallback ? "Offline Solo (fallback)" : "Offline Solo");
}

function clearLiveState() {
  for (const [, visual] of state.cars) {
    scene.remove(visual);
  }
  state.cars.clear();
  state.localCarState = null;
  state.lastEventSig = "";
  eventEl.textContent = "";
  playersEl.textContent = "0";
}

async function pollForAssignment(ticketID) {
  for (let i = 0; i < 60; i += 1) {
    const res = await requestJSON(`${state.gatewayURL}/v1/matchmaking/poll?ticket_id=${encodeURIComponent(ticketID)}`, {
      method: "GET",
      headers: authHeaders(),
    });

    if (res.status === "matched" && res.assignment) {
      return res.assignment;
    }
    setStatus(`Queueing... ${Math.min(i + 1, 60)}s`);
    await sleep(1000);
  }
  throw new Error("matchmaking timeout");
}

function connectWebSocket(rawServerAddr) {
  return new Promise((resolve, reject) => {
    const wsURL = resolveWebSocketURL(rawServerAddr);
    const url = `${wsURL}?player_id=${encodeURIComponent(state.playerID)}&display_name=${encodeURIComponent(state.displayName)}`;

    const ws = new WebSocket(url);
    let opened = false;

    ws.onopen = () => {
      opened = true;
      state.ws = ws;
      state.connected = true;
      setStatus("Connected");
      resolve();
    };

    ws.onerror = () => {
      if (!opened) {
        reject(new Error("websocket connection failed"));
      }
    };

    ws.onclose = () => {
      state.connected = false;
      if (state.mode === "online") {
        setStatus("Disconnected");
        menu.style.display = "block";
      }
    };

    ws.onmessage = (evt) => {
      try {
        const envelope = JSON.parse(evt.data);
        handleServerEnvelope(envelope);
      } catch (err) {
        console.warn("bad message", err);
      }
    };
  });
}

function setStartButtonsBusy(busy) {
  startOnlineBtn.disabled = busy;
  startOfflineBtn.disabled = busy;
}

function closeOnlineSocket() {
  if (state.ws) {
    try {
      state.ws.onclose = null;
      state.ws.close();
    } catch (_err) {
      // Ignore close failures.
    }
  }
  state.ws = null;
  state.connected = false;
}

function stopOfflineMode() {
  state.offline.active = false;
  state.offline.accumulator = 0;
  state.offline.matchState = null;
  state.offline.ctx = {};
}

function createOfflineSession(displayName, playerID) {
  const now = Date.now();
  const local = createOfflineCarState(playerID, displayName, "orange", false, -2048, 0, 0);
  const bot = createOfflineCarState("bot_offline", "Velocity Bot", "blue", true, 2048, 0, 180);
  return {
    active: true,
    accumulator: 0,
    matchState: {
      match_id: `offline_${now}`,
      tick: 0,
      created_at: new Date(now).toISOString(),
      cars: {
        [local.player_id]: local,
        [bot.player_id]: bot,
      },
      ball: {
        position: { x: 0, y: 0, z: BALL_RADIUS_UU + 20 },
        velocity: { x: 0, y: 0, z: 0 },
        radius: BALL_RADIUS_UU,
      },
      score: {
        orange: 0,
        blue: 0,
        time_remaining_ms: 300000,
      },
      events: [{ type: "kickoff", team: "orange", occurred_ms: now }],
    },
    ctx: {
      [local.player_id]: newOfflineJumpCtx(),
      [bot.player_id]: newOfflineJumpCtx(),
    },
    lastShotByTeam: { orange: 0, blue: 0 },
  };
}

function createOfflineCarState(playerID, displayName, team, isBot, x, y, yaw) {
  return {
    player_id: playerID,
    display_name: displayName,
    team,
    is_bot: isBot,
    position: { x, y, z: CAR_RADIUS_UU },
    velocity: { x: 0, y: 0, z: 0 },
    rotation: { pitch: 0, yaw, roll: 0 },
    boost: 100,
    is_grounded: true,
    last_input: {
      player_id: playerID,
      sequence: 0,
      throttle: 0,
      steer: 0,
      boost: false,
      jump: false,
      handbrake: false,
      client_ms: Date.now(),
    },
  };
}

function newOfflineJumpCtx() {
  return {
    usedJumps: 0,
    timeSinceJump: 0,
    holdTime: 0,
    stickyTime: 0,
    prevJump: false,
  };
}

function stepOfflineSimulation(dt) {
  if (!state.offline.active || !state.offline.matchState) {
    return;
  }

  state.offline.accumulator += dt;
  const step = 1 / OFFLINE_TICK_HZ;
  let guard = 0;
  while (state.offline.accumulator >= step && guard < 8) {
    runOfflineTick(step);
    state.offline.accumulator -= step;
    guard += 1;
  }
  applyMatchState(state.offline.matchState);
}

function runOfflineTick(dt) {
  const session = state.offline;
  const m = session.matchState;
  if (!m) {
    return;
  }

  m.tick += 1;
  m.events = [];

  if (m.score.time_remaining_ms > 0) {
    m.score.time_remaining_ms = Math.max(0, m.score.time_remaining_ms - Math.max(1, Math.round(dt * 1000)));
  }

  const localCar = m.cars[state.localCarID];
  const botCar = m.cars.bot_offline;
  if (!localCar || !botCar) {
    return;
  }

  const localInput = readControlState();
  const botInput = offlineBotInput(botCar, m.ball);

  updateOfflineCar(localCar, localInput, session.ctx[localCar.player_id], dt);
  updateOfflineCar(botCar, botInput, session.ctx[botCar.player_id], dt);

  clampOfflineCarBounds(localCar);
  clampOfflineCarBounds(botCar);

  updateOfflineBall(m.ball, dt);
  clampOfflineBallBounds(m.ball);
  resolveOfflineCarBallCollision(localCar, m.ball);
  resolveOfflineCarBallCollision(botCar, m.ball);

  const now = Date.now();
  if (m.ball.position.x > ARENA_LENGTH_UU * 0.35 && m.ball.velocity.x > 200 && Math.abs(m.ball.position.y) <= GOAL_WIDTH_UU * 0.7) {
    if (now - session.lastShotByTeam.orange >= 700) {
      m.events.push({ type: "shot_on_goal", team: "orange", occurred_ms: now });
      session.lastShotByTeam.orange = now;
    }
  }
  if (m.ball.position.x < -ARENA_LENGTH_UU * 0.35 && m.ball.velocity.x < -200 && Math.abs(m.ball.position.y) <= GOAL_WIDTH_UU * 0.7) {
    if (now - session.lastShotByTeam.blue >= 700) {
      m.events.push({ type: "shot_on_goal", team: "blue", occurred_ms: now });
      session.lastShotByTeam.blue = now;
    }
  }

  const inGoalY = Math.abs(m.ball.position.y) <= GOAL_WIDTH_UU / 2;
  const inGoalZ = m.ball.position.z <= GOAL_HEIGHT_UU;
  if (inGoalY && inGoalZ) {
    if (m.ball.position.x >= ARENA_LENGTH_UU / 2) {
      m.score.orange += 1;
      m.events.push({ type: "goal", team: "orange", occurred_ms: now });
      resetOfflineKickoff("orange");
    } else if (m.ball.position.x <= -ARENA_LENGTH_UU / 2) {
      m.score.blue += 1;
      m.events.push({ type: "goal", team: "blue", occurred_ms: now });
      resetOfflineKickoff("blue");
    }
  }
}

function resetOfflineKickoff(scoringTeam) {
  const session = state.offline;
  const m = session.matchState;
  if (!m) {
    return;
  }
  m.ball.position = { x: 0, y: 0, z: BALL_RADIUS_UU + 20 };
  m.ball.velocity = { x: 0, y: 0, z: 0 };
  const now = Date.now();
  for (const car of Object.values(m.cars)) {
    const isOrange = car.team === "orange";
    car.position.x = isOrange ? -2048 : 2048;
    car.position.y = 0;
    car.position.z = CAR_RADIUS_UU;
    car.velocity.x = 0;
    car.velocity.y = 0;
    car.velocity.z = 0;
    car.rotation.yaw = isOrange ? 0 : 180;
    car.boost = 100;
    car.is_grounded = true;
    session.ctx[car.player_id] = newOfflineJumpCtx();
  }
  m.events.push({ type: "kickoff", team: scoringTeam, occurred_ms: now });
}

function offlineBotInput(car, ball) {
  const dx = ball.position.x - car.position.x;
  const dy = ball.position.y - car.position.y;
  const dz = ball.position.z - car.position.z;
  const dist = Math.hypot(dx, dy);
  const targetYaw = (Math.atan2(dy, dx) * 180) / Math.PI;
  const delta = normalizeSignedDeg(targetYaw - car.rotation.yaw);
  return {
    throttle: Math.abs(delta) > 120 ? -0.2 : 1,
    steer: clamp(delta / 35, -1, 1),
    boost: Math.abs(delta) < 10 && dist > 750 && car.boost > 20,
    jump: car.is_grounded && dist < 240 && dz > 120,
    handbrake: Math.abs(delta) > 75,
  };
}

function updateOfflineCar(car, input, ctx, dt) {
  const prevJump = !!ctx.prevJump;
  const jumpPressed = input.jump && !prevJump;
  let yawRad = (car.rotation.yaw * Math.PI) / 180;

  const speed2D = Math.hypot(car.velocity.x, car.velocity.y);
  let turnScale = 1 - Math.min(speed2D / MAX_CAR_SPEED, 0.75);
  let turnRate = TURN_RATE * (0.55 + turnScale);
  if (input.handbrake && car.is_grounded) {
    turnRate *= HANDBRAKE_TURN_BOOST;
  }
  car.rotation.yaw = normalizeDeg(car.rotation.yaw + ((input.steer * turnRate * dt * 180) / Math.PI));
  yawRad = (car.rotation.yaw * Math.PI) / 180;

  const forwardX = Math.cos(yawRad);
  const forwardY = Math.sin(yawRad);
  const rightX = -forwardY;
  const rightY = forwardX;

  let forwardSpeed = car.velocity.x * forwardX + car.velocity.y * forwardY;
  let lateralSpeed = car.velocity.x * rightX + car.velocity.y * rightY;

  let accel = 0;
  if (car.is_grounded) {
    accel = input.throttle * THROTTLE_ACCEL;
    if (input.throttle * forwardSpeed < 0) {
      accel = input.throttle * BRAKE_ACCEL;
    }
  } else if (input.throttle >= 0) {
    accel = input.throttle * AIR_THROTTLE_ACCEL;
  } else {
    accel = input.throttle * AIR_REVERSE_ACCEL;
  }
  forwardSpeed += accel * dt;

  const usingBoost = input.boost && car.boost > 0 && input.throttle > 0;
  if (usingBoost) {
    forwardSpeed += BOOST_ACCEL * dt;
    car.boost = Math.max(0, car.boost - 34 * dt);
  } else {
    car.boost = Math.min(100, car.boost + 8 * dt);
  }

  if (Math.abs(input.throttle) < 0.05 && car.is_grounded) {
    forwardSpeed *= COAST_FRICTION;
  }
  const maxSpeed = usingBoost ? MAX_CAR_SPEED : MAX_DRIVE_SPEED;
  forwardSpeed = clamp(forwardSpeed, -MAX_CAR_SPEED, maxSpeed);

  if (car.is_grounded) {
    const grip = input.handbrake ? HANDBRAKE_GRIP : LATERAL_GRIP;
    lateralSpeed *= grip;
  } else {
    lateralSpeed *= 0.985;
  }

  car.velocity.x = forwardX * forwardSpeed + rightX * lateralSpeed;
  car.velocity.y = forwardY * forwardSpeed + rightY * lateralSpeed;

  let didFirstJump = false;
  if (car.is_grounded) {
    ctx.usedJumps = 0;
    ctx.timeSinceJump = 0;
    ctx.holdTime = 0;
    ctx.stickyTime = 0;
  }

  if (jumpPressed && ctx.usedJumps === 0 && car.is_grounded) {
    car.velocity.z += JUMP_VELOCITY;
    car.is_grounded = false;
    ctx.usedJumps = 1;
    ctx.timeSinceJump = 0;
    ctx.holdTime = 0;
    ctx.stickyTime = STICKY_TIME;
    didFirstJump = true;
  }

  if (ctx.usedJumps > 0 && !car.is_grounded) {
    ctx.timeSinceJump += dt;
    if (input.jump && ctx.holdTime < JUMP_HOLD_MAX && ctx.usedJumps === 1) {
      car.velocity.z += JUMP_HOLD_ACCEL * dt;
      ctx.holdTime += dt;
    }
    if (ctx.stickyTime > 0) {
      car.velocity.z -= STICKY_FORCE * dt;
      ctx.stickyTime -= dt;
    }
    if (jumpPressed && !didFirstJump && ctx.usedJumps === 1 && ctx.timeSinceJump <= DOUBLE_JUMP_MAX) {
      car.velocity.z += JUMP_VELOCITY;
      let dodgeX = forwardX * input.throttle + rightX * input.steer;
      let dodgeY = forwardY * input.throttle + rightY * input.steer;
      let mag = Math.hypot(dodgeX, dodgeY);
      if (mag < 0.1) {
        dodgeX = forwardX;
        dodgeY = forwardY;
        mag = 1;
      }
      dodgeX /= mag;
      dodgeY /= mag;
      car.velocity.x += dodgeX * 500;
      car.velocity.y += dodgeY * 500;
      ctx.usedJumps = 2;
      ctx.holdTime = JUMP_HOLD_MAX;
      ctx.stickyTime = 0;
    }
  }

  car.velocity.z += GRAVITY * dt;
  if (car.is_grounded) {
    car.velocity.x *= GROUND_FRICTION;
    car.velocity.y *= GROUND_FRICTION;
  } else {
    car.velocity.x *= AIR_RESISTANCE;
    car.velocity.y *= AIR_RESISTANCE;
  }

  car.position.x += car.velocity.x * dt;
  car.position.y += car.velocity.y * dt;
  car.position.z += car.velocity.z * dt;

  if (car.position.z <= CAR_RADIUS_UU) {
    car.position.z = CAR_RADIUS_UU;
    if (car.velocity.z < 0) {
      car.velocity.z = 0;
    }
    car.is_grounded = true;
    ctx.stickyTime = 0;
  }
  if (car.position.z > ARENA_HEIGHT_UU - CAR_RADIUS_UU) {
    car.position.z = ARENA_HEIGHT_UU - CAR_RADIUS_UU;
    car.velocity.z *= -0.25;
  }

  ctx.prevJump = input.jump;
  car.last_input = {
    player_id: car.player_id,
    sequence: car.last_input?.sequence ? car.last_input.sequence + 1 : 1,
    throttle: input.throttle,
    steer: input.steer,
    boost: input.boost,
    jump: input.jump,
    handbrake: input.handbrake,
    client_ms: Date.now(),
  };
}

function clampOfflineCarBounds(car) {
  const halfL = ARENA_LENGTH_UU / 2;
  const halfW = ARENA_WIDTH_UU / 2;
  if (car.position.x < -halfL + CAR_RADIUS_UU) {
    car.position.x = -halfL + CAR_RADIUS_UU;
    car.velocity.x *= -0.3;
  }
  if (car.position.x > halfL - CAR_RADIUS_UU) {
    car.position.x = halfL - CAR_RADIUS_UU;
    car.velocity.x *= -0.3;
  }
  if (car.position.y < -halfW + CAR_RADIUS_UU) {
    car.position.y = -halfW + CAR_RADIUS_UU;
    car.velocity.y *= -0.3;
  }
  if (car.position.y > halfW - CAR_RADIUS_UU) {
    car.position.y = halfW - CAR_RADIUS_UU;
    car.velocity.y *= -0.3;
  }
}

function updateOfflineBall(ball, dt) {
  ball.velocity.z += GRAVITY * dt;
  ball.position.x += ball.velocity.x * dt;
  ball.position.y += ball.velocity.y * dt;
  ball.position.z += ball.velocity.z * dt;

  if (ball.position.z <= BALL_RADIUS_UU + 8) {
    ball.velocity.x *= 0.9975;
    ball.velocity.y *= 0.9975;
  } else {
    ball.velocity.x *= 0.9995;
    ball.velocity.y *= 0.9995;
  }
  ball.velocity.z *= 0.9994;

  const speed = Math.hypot(ball.velocity.x, ball.velocity.y, ball.velocity.z);
  if (speed > BALL_MAX_SPEED && speed > 0) {
    const s = BALL_MAX_SPEED / speed;
    ball.velocity.x *= s;
    ball.velocity.y *= s;
    ball.velocity.z *= s;
  }
}

function clampOfflineBallBounds(ball) {
  const halfL = ARENA_LENGTH_UU / 2;
  const halfW = ARENA_WIDTH_UU / 2;
  if (ball.position.z < BALL_RADIUS_UU) {
    ball.position.z = BALL_RADIUS_UU;
    ball.velocity.z = -ball.velocity.z * BALL_RESTITUTION;
  }
  if (ball.position.z > ARENA_HEIGHT_UU - BALL_RADIUS_UU) {
    ball.position.z = ARENA_HEIGHT_UU - BALL_RADIUS_UU;
    ball.velocity.z = -ball.velocity.z * BALL_RESTITUTION;
  }

  const inGoalY = Math.abs(ball.position.y) <= GOAL_WIDTH_UU / 2;
  const inGoalZ = ball.position.z <= GOAL_HEIGHT_UU;
  if (!inGoalY || !inGoalZ) {
    if (ball.position.x < -halfL + BALL_RADIUS_UU) {
      ball.position.x = -halfL + BALL_RADIUS_UU;
      ball.velocity.x = -ball.velocity.x * WALL_RESTITUTION;
    }
    if (ball.position.x > halfL - BALL_RADIUS_UU) {
      ball.position.x = halfL - BALL_RADIUS_UU;
      ball.velocity.x = -ball.velocity.x * WALL_RESTITUTION;
    }
  }
  if (ball.position.y < -halfW + BALL_RADIUS_UU) {
    ball.position.y = -halfW + BALL_RADIUS_UU;
    ball.velocity.y = -ball.velocity.y * WALL_RESTITUTION;
  }
  if (ball.position.y > halfW - BALL_RADIUS_UU) {
    ball.position.y = halfW - BALL_RADIUS_UU;
    ball.velocity.y = -ball.velocity.y * WALL_RESTITUTION;
  }
}

function resolveOfflineCarBallCollision(car, ball) {
  const dx = ball.position.x - car.position.x;
  const dy = ball.position.y - car.position.y;
  const dz = ball.position.z - car.position.z;
  const dist = Math.hypot(dx, dy, dz);
  const minDist = CAR_RADIUS_UU + BALL_RADIUS_UU;
  if (dist <= 0 || dist >= minDist) {
    return;
  }

  const nx = dx / dist;
  const ny = dy / dist;
  const nz = dz / dist;
  const carDot = car.velocity.x * nx + car.velocity.y * ny + car.velocity.z * nz;
  const ballDot = ball.velocity.x * nx + ball.velocity.y * ny + ball.velocity.z * nz;
  const rel = ballDot - carDot;
  if (rel > 0) {
    return;
  }

  const impulse = -(1 + CAR_BALL_ELASTICITY) * rel;
  ball.velocity.x += impulse * nx;
  ball.velocity.y += impulse * ny;
  ball.velocity.z += impulse * nz;

  const overlap = minDist - dist;
  ball.position.x += nx * overlap * 0.85;
  ball.position.y += ny * overlap * 0.85;
  ball.position.z += nz * overlap * 0.85;

  car.position.x -= nx * overlap * 0.15;
  car.position.y -= ny * overlap * 0.15;
  car.position.z -= nz * overlap * 0.15;
}

function handleServerEnvelope(envelope) {
  switch (envelope.type) {
    case "welcome":
    case "state":
      if (envelope.state) {
        applyMatchState(envelope.state);
      }
      break;
    case "pong":
      if (state.pingSentAt > 0) {
        state.pingMS = Math.round(performance.now() - state.pingSentAt);
        pingEl.textContent = `${state.pingMS}`;
      }
      break;
    case "error":
      setStatus(`Server error: ${envelope.message || "unknown"}`);
      break;
    default:
      break;
  }
}

function applyMatchState(matchState) {
  scoreOrangeEl.textContent = String(matchState.score.orange ?? 0);
  scoreBlueEl.textContent = String(matchState.score.blue ?? 0);
  timerEl.textContent = formatTimer(matchState.score.time_remaining_ms ?? 0);

  const cars = matchState.cars || {};
  const seen = new Set();

  for (const [carID, carState] of Object.entries(cars)) {
    seen.add(carID);

    let visual = state.cars.get(carID);
    if (!visual) {
      visual = createCarVisual(carState);
      state.cars.set(carID, visual);
    }

    const leadSeconds = carID === state.localCarID
      ? Math.min(((state.pingMS ?? 28) / 1000) * 0.7, 0.07)
      : 0.02;
    const predictedPos = {
      x: carState.position.x + carState.velocity.x * leadSeconds,
      y: carState.position.y + carState.velocity.y * leadSeconds,
      z: carState.position.z + carState.velocity.z * leadSeconds,
    };
    visual.userData.targetPos.copy(toScenePos(predictedPos));
    visual.userData.targetRotY = (carState.rotation.yaw * Math.PI) / 180;
    visual.userData.velX = carState.velocity.x;
    visual.userData.velY = carState.velocity.y;

    if (carID === state.localCarID) {
      state.localCarState = carState;
      boostEl.textContent = `${Math.round(carState.boost || 0)}`;
    }
  }

  for (const [id, visual] of state.cars.entries()) {
    if (!seen.has(id)) {
      scene.remove(visual);
      state.cars.delete(id);
    }
  }

  playersEl.textContent = String(state.cars.size);

  if (state.ballVisual) {
    if (!state.ballVisual.userData.targetPos) {
      state.ballVisual.userData.targetPos = new THREE.Vector3();
    }
    state.ballVisual.userData.targetPos.copy(toScenePos(matchState.ball.position));
  }

  if (Array.isArray(matchState.events) && matchState.events.length > 0) {
    const ev = matchState.events[matchState.events.length - 1];
    const sig = `${ev.type}|${ev.team || ""}|${ev.occurred_ms || 0}`;
    if (sig !== state.lastEventSig) {
      state.lastEventSig = sig;
      showEvent(labelForEvent(ev));
    }
  }
}

function labelForEvent(ev) {
  if (!ev || !ev.type) {
    return "";
  }
  if (ev.type === "goal") {
    return `${(ev.team || "").toUpperCase()} GOAL`;
  }
  if (ev.type === "shot_on_goal") {
    return "SHOT ON GOAL";
  }
  if (ev.type === "kickoff") {
    return "KICKOFF";
  }
  if (ev.type === "player_join") {
    return "PLAYER JOINED";
  }
  return ev.type.replaceAll("_", " ").toUpperCase();
}

function showEvent(text) {
  eventEl.textContent = text;
  if (!text) {
    return;
  }
  setTimeout(() => {
    if (eventEl.textContent === text) {
      eventEl.textContent = "";
    }
  }, HUD_EVENT_TIMEOUT_MS);
}

function formatTimer(ms) {
  const totalSec = Math.max(Math.floor(ms / 1000), 0);
  const min = Math.floor(totalSec / 60)
    .toString()
    .padStart(2, "0");
  const sec = (totalSec % 60).toString().padStart(2, "0");
  return `${min}:${sec}`;
}

function toScenePos(v) {
  return new THREE.Vector3(v.x * SIM_SCALE, v.z * SIM_SCALE, v.y * SIM_SCALE);
}

function lerpAngle(a, b, t) {
  const diff = Math.atan2(Math.sin(b - a), Math.cos(b - a));
  return a + diff * t;
}

function clamp(v, minV, maxV) {
  return Math.max(minV, Math.min(maxV, v));
}

function normalizeDeg(v) {
  let out = v;
  while (out >= 360) out -= 360;
  while (out < 0) out += 360;
  return out;
}

function normalizeSignedDeg(v) {
  let out = v;
  while (out > 180) out -= 360;
  while (out < -180) out += 360;
  return out;
}

function setStatus(text) {
  statusEl.textContent = text;
}

function authHeaders() {
  return {
    "Content-Type": "application/json",
    Authorization: `Bearer ${state.token}`,
  };
}

async function requestJSON(url, init) {
  const opts = {
    ...init,
    headers: {
      ...(init?.headers || {}),
    },
  };
  if (!opts.headers["Content-Type"] && opts.body) {
    opts.headers["Content-Type"] = "application/json";
  }

  const res = await fetch(url, opts);
  const text = await res.text();
  let payload = {};
  try {
    payload = text ? JSON.parse(text) : {};
  } catch (_err) {
    throw new Error(`invalid json from ${url}`);
  }

  if (!res.ok) {
    const message = payload.error || payload.message || `http ${res.status}`;
    throw new Error(message);
  }
  return payload;
}

function resolveWebSocketURL(inputURL) {
  if (!inputURL) {
    return "ws://localhost:9003/ws";
  }

  try {
    const u = new URL(inputURL);
    const hostFromBrowser = window.location.hostname;
    if (u.hostname === "gameserver") {
      u.hostname = hostFromBrowser || "localhost";
    }
    if (u.hostname === "0.0.0.0") {
      u.hostname = hostFromBrowser || "localhost";
    }
    return u.toString();
  } catch (_err) {
    return inputURL;
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function ballRadiusVisual() {
  return 91.25 * SIM_SCALE;
}
