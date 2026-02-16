import * as THREE from "https://cdn.jsdelivr.net/npm/three@0.167.1/build/three.module.js";

const SIM_SCALE = 0.01;
const HUD_EVENT_TIMEOUT_MS = 1200;
const INPUT_SEND_HZ = 60;
const ARENA_LENGTH_UU = 8192;
const ARENA_WIDTH_UU = 10240;
const ARENA_HEIGHT_UU = 2044;
const GOAL_WIDTH_UU = 1785.51;
const GOAL_HEIGHT_UU = 642.775;

const canvas = document.getElementById("game-canvas");
const menu = document.getElementById("menu");
const startBtn = document.getElementById("start-btn");
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
  lastInput: null,
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

startBtn.addEventListener("click", async () => {
  await startMatchFlow();
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
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
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
    if (!state.ws || state.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    state.pingSentAt = performance.now();
    state.ws.send(JSON.stringify({ type: "ping" }));
  }, 2000);
}

function buildInputPayload() {
  const throttle = (keys.has("KeyW") || keys.has("ArrowUp") ? 1 : 0) + (keys.has("KeyS") || keys.has("ArrowDown") ? -1 : 0);
  const steer = (keys.has("KeyD") || keys.has("ArrowRight") ? 1 : 0) + (keys.has("KeyA") || keys.has("ArrowLeft") ? -1 : 0);

  const payload = {
    player_id: state.playerID,
    sequence: state.seq++,
    throttle: clamp(throttle, -1, 1),
    steer: clamp(steer, -1, 1),
    boost: keys.has("ShiftLeft") || keys.has("ShiftRight"),
    jump: keys.has("Space"),
    handbrake: keys.has("ControlLeft") || keys.has("ControlRight"),
    client_ms: Date.now(),
  };
  state.lastInput = payload;
  return payload;
}

async function startMatchFlow() {
  if (startBtn.disabled) {
    return;
  }

  try {
    startBtn.disabled = true;
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
    setStatus(`Error: ${err.message || "match start failed"}`);
    menu.style.display = "block";
  } finally {
    startBtn.disabled = false;
  }
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
      setStatus("Disconnected");
      menu.style.display = "block";
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
