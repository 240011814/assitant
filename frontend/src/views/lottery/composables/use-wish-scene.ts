import * as THREE from 'three';
import { EffectComposer } from 'three/examples/jsm/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/examples/jsm/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';
import { OutputPass } from 'three/examples/jsm/postprocessing/OutputPass.js';
import { rarityTheme } from './rarity-theme';
import skyCloudsUrl from '@/assets/imgs/lottery/sky-clouds.jpg';

/**
 * 原神祈愿风 three.js 场景控制器 (仅被 gacha-animation-modal.vue 使用):
 * 星空穿越 → 流星 → 闪光 → 光柱+旋转光线揭示 → 结果氛围, 全程 UnrealBloom 辉光。
 * 阶段由 use-gacha-animation 的状态机驱动 (setPhase), 场景只负责渲染与阶段过渡动画。
 * 弹窗关闭必须调用 dispose() 释放 GPU 资源。
 */
export type WishPhase = 'idle' | 'meteor' | 'flash' | 'reveal' | 'result';

/** 移动端降配: 减粒子数 + 降像素比 */
function isLowEnd() {
  return window.innerWidth < 640;
}

/** Canvas 生成径向辉光贴图 (白心→主题色→透明), 供 sprite/points 共用 */
function makeGlowTexture(size = 128, inner = 'rgba(255,255,255,1)', mid = 'rgba(255,255,255,0.35)'): THREE.Texture {
  const c = document.createElement('canvas');
  c.width = size;
  c.height = size;
  const ctx = c.getContext('2d')!;
  const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
  g.addColorStop(0, inner);
  g.addColorStop(0.35, mid);
  g.addColorStop(1, 'rgba(255,255,255,0)');
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, size, size);
  const tex = new THREE.CanvasTexture(c);
  tex.colorSpace = THREE.SRGBColorSpace;
  return tex;
}

/** 极坐标旋转光线 shader: 原神"祈愿门"标志性放射光, 两层反向旋转叠加 */
function makeRaysMaterial(color: THREE.Color, rayCount: number, speed: number, sharpness: number) {
  return new THREE.ShaderMaterial({
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    uniforms: {
      uTime: { value: 0 },
      uIntensity: { value: 0 },
      uColor: { value: color },
      uRayCount: { value: rayCount },
      uSpeed: { value: speed },
      uSharp: { value: sharpness }
    },
    vertexShader: /* glsl */ `
      varying vec2 vUv;
      void main() {
        vUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: /* glsl */ `
      uniform float uTime;
      uniform float uIntensity;
      uniform vec3 uColor;
      uniform float uRayCount;
      uniform float uSpeed;
      uniform float uSharp;
      varying vec2 vUv;
      void main() {
        vec2 p = vUv - 0.5;
        float ang = atan(p.y, p.x);
        // 光线条纹: 角度调制 + 时间旋转; pow 控制锐利度
        float rays = pow(abs(sin((ang + uTime * uSpeed) * uRayCount)), uSharp);
        // 空心中心 + 边缘衰减
        float r = length(p) * 2.0;
        float hollow = smoothstep(0.06, 0.3, r);
        float edge = smoothstep(1.0, 0.45, r);
        float a = rays * hollow * edge * uIntensity;
        gl_FragColor = vec4(uColor * a, a);
      }
    `
  });
}

export interface WishSceneHandle {
  setPhase(phase: WishPhase): void;
  setRarity(rarity: number): void;
  dispose(): void;
}

/** 揭晓前的中性配色 (原神同款: 流星阶段不泄露稀有度, 蓝白星流一视同仁) */
const NEUTRAL = { primary: '#7fa8ff', secondary: '#d5e4ff' };

export function createWishScene(canvas: HTMLCanvasElement, initialRarity: number): WishSceneHandle {
  const lowEnd = isLowEnd();
  // 揭晓前统一中性蓝白; 真实主题暂存, reveal 进入时才"炸色"
  const pendingRarity = initialRarity;
  const primary = new THREE.Color(NEUTRAL.primary);
  const secondary = new THREE.Color(NEUTRAL.secondary);

  // ==================== 基础三件套 + 辉光后期 ====================
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: !lowEnd, alpha: false });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, lowEnd ? 1.5 : 2));

  const scene = new THREE.Scene();
  const bgColor = new THREE.Color('#070b1e').lerp(primary, 0.07);
  scene.background = bgColor;

  const camera = new THREE.PerspectiveCamera(60, 1, 0.1, 300);
  camera.position.set(0, 0, 27);

  const composer = new EffectComposer(renderer);
  composer.addPass(new RenderPass(scene, camera));
  // threshold 调高: 只有真正的高亮核心才辉光, 避免金色主题整屏过曝
  const bloom = new UnrealBloomPass(new THREE.Vector2(1, 1), 0.55, 0.5, 0.45);
  composer.addPass(bloom);
  composer.addPass(new OutputPass());

  const disposables: Array<{ dispose: () => void }> = [];
  const track = <T extends { dispose: () => void }>(d: T): T => {
    disposables.push(d);
    return d;
  };

  // ==================== 星空 (穿越感 + 逐星闪烁 shader) ====================
  const starCount = lowEnd ? 420 : 900;
  const starGeo = track(new THREE.BufferGeometry());
  const starPos = new Float32Array(starCount * 3);
  const starPhase = new Float32Array(starCount);
  const starSize = new Float32Array(starCount);
  for (let i = 0; i < starCount; i += 1) {
    starPos[i * 3] = (Math.random() - 0.5) * 130;
    starPos[i * 3 + 1] = (Math.random() - 0.5) * 90;
    starPos[i * 3 + 2] = -Math.random() * 140 + 12;
    starPhase[i] = Math.random();
    starSize[i] = 0.9 + Math.random() * 1.5;
  }
  starGeo.setAttribute('position', new THREE.BufferAttribute(starPos, 3));
  starGeo.setAttribute('aPhase', new THREE.BufferAttribute(starPhase, 1));
  starGeo.setAttribute('aSize', new THREE.BufferAttribute(starSize, 1));
  const starTex = track(makeGlowTexture(64, 'rgba(255,255,255,1)', 'rgba(255,255,255,0.5)'));
  const starMat = track(
    new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      uniforms: {
        uTime: { value: 0 },
        uPixelRatio: { value: 1 },
        uColor: { value: new THREE.Color('#cfe3ff').lerp(primary, 0.25) }
      },
      vertexShader: /* glsl */ `
        attribute float aPhase;
        attribute float aSize;
        uniform float uTime;
        uniform float uPixelRatio;
        varying float vTw;
        void main() {
          // 逐星异相闪烁
          vTw = 0.55 + 0.45 * sin(uTime * (0.6 + aPhase * 1.7) + aPhase * 40.0);
          vec4 mv = modelViewMatrix * vec4(position, 1.0);
          gl_PointSize = aSize * uPixelRatio * (90.0 / -mv.z) * (0.8 + 0.4 * vTw);
          gl_Position = projectionMatrix * mv;
        }
      `,
      fragmentShader: /* glsl */ `
        uniform vec3 uColor;
        varying float vTw;
        void main() {
          float d = length(gl_PointCoord - 0.5);
          float a = smoothstep(0.5, 0.06, d) * vTw;
          gl_FragColor = vec4(uColor * a, a);
        }
      `
    })
  );
  const stars = new THREE.Points(starGeo, starMat);
  scene.add(stars);

  // ==================== 天空背景图 (云海星空照片, 随稀有度调色) ====================
  const skyTex = track(new THREE.TextureLoader().load(skyCloudsUrl));
  skyTex.colorSpace = THREE.SRGBColorSpace;
  const skyTint = new THREE.Color('#8fa8d0').multiplyScalar(0.5);
  const skyMat = track(new THREE.MeshBasicMaterial({ map: skyTex, color: skyTint.clone(), depthWrite: false }));
  // 尺寸覆盖所有宽高比 (最远端可视范围 ~124 高 / 300 宽 @21:9)
  const skyPlane = new THREE.Mesh(track(new THREE.PlaneGeometry(330, 185)), skyMat);
  skyPlane.position.z = -82;
  scene.add(skyPlane);

  // ==================== 流星 (3D 胶囊 Mesh + 渐变 Shader: 头亮尾隐, 沿轨迹穿越 3D 空间) ====================
  const cometGroup = new THREE.Group();
  cometGroup.visible = false;
  scene.add(cometGroup);

  const cometCoreTex = track(makeGlowTexture());
  const COMET_LOCAL_HALF = 2.0; // 胶囊局部 Y 半跨度 (radius 0.55 + length/2 1.45)
  const cometMat = track(
    new THREE.ShaderMaterial({
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
      uniforms: {
        uColor: { value: new THREE.Color(NEUTRAL.primary) },
        uTime: { value: 0 },
        uOpacity: { value: 1 }
      },
      vertexShader: /* glsl */ `
        varying float vLocalY;
        varying vec3 vNormal;
        varying vec3 vViewDir;
        void main() {
          vLocalY = position.y / ${COMET_LOCAL_HALF.toFixed(1)};
          vNormal = normalMatrix * normal;
          vec4 mv = modelViewMatrix * vec4(position, 1.0);
          vViewDir = -mv.xyz;
          gl_Position = projectionMatrix * mv;
        }
      `,
      fragmentShader: /* glsl */ `
        uniform vec3 uColor;
        uniform float uTime;
        uniform float uOpacity;
        varying float vLocalY;
        varying vec3 vNormal;
        varying vec3 vViewDir;
        void main() {
          // t: 0=头部(-Y) → 1=尾端(+Y)
          float t = clamp(vLocalY * 0.5 + 0.5, 0.0, 1.0);
          // 朝向相机的面更亮 (球体感), 叠加时间微颤
          float body = pow(max(dot(normalize(vNormal), normalize(vViewDir)), 0.0), 1.3);
          float flicker = 0.92 + 0.08 * sin(uTime * 30.0 + t * 12.0);
          // 头部恒亮, 向尾端渐隐
          float tailFade = mix(1.0, pow(1.0 - t, 1.5), 0.85);
          float a = uOpacity * body * tailFade * flicker;
          vec3 col = mix(vec3(1.0), uColor, t * 0.85);
          gl_FragColor = vec4(col * a, a);
        }
      `
    })
  );
  const cometMesh = new THREE.Mesh(track(new THREE.CapsuleGeometry(0.55, 2.9, 6, 24)), cometMat);
  cometGroup.add(cometMesh);

  // 头部辉光 sprite (叠在 Mesh 头上, bloom 加强核心)
  const cometGlowMat = track(new THREE.SpriteMaterial({ map: cometCoreTex, color: new THREE.Color(NEUTRAL.primary), transparent: true, opacity: 0.9, depthWrite: false, blending: THREE.AdditiveBlending }));
  const cometGlow = new THREE.Sprite(cometGlowMat);
  cometGlow.scale.setScalar(2.6);
  cometGroup.add(cometGlow);

  // 尾迹粒子池 (additive + 顶点色衰减到黑即隐形)
  const trailCount = lowEnd ? 120 : 240;
  const trailGeo = track(new THREE.BufferGeometry());
  trailGeo.setAttribute('position', new THREE.BufferAttribute(new Float32Array(trailCount * 3), 3));
  trailGeo.setAttribute('color', new THREE.BufferAttribute(new Float32Array(trailCount * 3), 3));
  const trailMat = track(
    new THREE.PointsMaterial({ map: starTex, size: 0.7, vertexColors: true, transparent: true, depthWrite: false, blending: THREE.AdditiveBlending })
  );
  const trailPoints = new THREE.Points(trailGeo, trailMat);
  scene.add(trailPoints);
  const trailData = Array.from({ length: trailCount }, () => ({ life: 0, vx: 0, vy: 0, vz: 0 }));
  let trailCursor = 0;

  // ==================== 祈愿门: 双层旋转光线 ====================
  const raysGroup = new THREE.Group();
  raysGroup.visible = false;
  scene.add(raysGroup);
  const raysMatA = track(makeRaysMaterial(primary.clone(), 12, 0.5, 6));
  const raysMatB = track(makeRaysMaterial(secondary.clone().lerp(new THREE.Color('#ffffff'), 0.3), 24, -0.9, 12));
  const raysPlaneA = new THREE.Mesh(track(new THREE.PlaneGeometry(90, 90)), raysMatA);
  const raysPlaneB = new THREE.Mesh(track(new THREE.PlaneGeometry(90, 90)), raysMatB);
  raysPlaneA.position.z = -6;
  raysPlaneB.position.z = -6.5;
  raysGroup.add(raysPlaneA, raysPlaneB);

  // ==================== 光柱 ====================
  const pillarMat = new THREE.ShaderMaterial({
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
    uniforms: {
      uIntensity: { value: 0 },
      uColor: { value: primary.clone() },
      uTime: { value: 0 }
    },
    vertexShader: /* glsl */ `
      varying vec2 vUv;
      void main() {
        vUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: /* glsl */ `
      uniform float uIntensity;
      uniform vec3 uColor;
      uniform float uTime;
      varying vec2 vUv;
      void main() {
        // 中央亮芯 + 向两侧衰减, 底部微弱呼吸
        float x = abs(vUv.x - 0.5) * 2.0;
        float core = pow(1.0 - x, 3.0);
        float shimmer = 0.85 + 0.15 * sin(vUv.y * 20.0 - uTime * 4.0);
        float fadeTop = smoothstep(1.0, 0.35, vUv.y);
        float a = core * fadeTop * uIntensity * shimmer;
        gl_FragColor = vec4(uColor * a, a);
      }
    `
  });
  track(pillarMat);
  const pillar = new THREE.Mesh(track(new THREE.PlaneGeometry(10, 60)), pillarMat);
  pillar.position.set(0, 0, -4);
  pillar.visible = false;
  scene.add(pillar);

  // ==================== 冲击波环 ×3 ====================
  const ringMats: THREE.MeshBasicMaterial[] = [];
  const rings: THREE.Mesh[] = [];
  for (let i = 0; i < 3; i += 1) {
    const mat = track(
      new THREE.MeshBasicMaterial({ color: i % 2 === 0 ? primary.clone() : secondary.clone(), transparent: true, opacity: 0, depthWrite: false, blending: THREE.AdditiveBlending, side: THREE.DoubleSide })
    );
    const ring = new THREE.Mesh(track(new THREE.RingGeometry(0.96, 1, 64)), mat);
    ring.visible = false;
    ring.position.z = -3 - i;
    scene.add(ring);
    ringMats.push(mat);
    rings.push(ring);
  }

  // ==================== 爆发粒子池 ====================
  const burstCount = lowEnd ? 260 : 600;
  const burstGeo = track(new THREE.BufferGeometry());
  const burstPositions = new Float32Array(burstCount * 3);
  const burstColors = new Float32Array(burstCount * 3);
  burstGeo.setAttribute('position', new THREE.BufferAttribute(burstPositions, 3));
  burstGeo.setAttribute('color', new THREE.BufferAttribute(burstColors, 3));
  const burstMat = track(
    new THREE.PointsMaterial({ map: starTex, size: 0.62, vertexColors: true, transparent: true, depthWrite: false, blending: THREE.AdditiveBlending })
  );
  const burstPoints = new THREE.Points(burstGeo, burstMat);
  burstPoints.visible = false;
  scene.add(burstPoints);
  const burstData = Array.from({ length: burstCount }, () => ({
    life: 0,
    maxLife: 1,
    vx: 0,
    vy: 0,
    vz: 0
  }));
  const burstBase = primary.clone();
  const burstBase2 = secondary.clone();

  /** reveal 进入时引爆粒子池 */
  function detonateBurst() {
    for (let i = 0; i < burstCount; i += 1) {
      const d = burstData[i];
      const theta = Math.random() * Math.PI * 2;
      const phi = Math.acos(2 * Math.random() - 1);
      const speed = 6 + Math.random() * 16;
      d.vx = Math.sin(phi) * Math.cos(theta) * speed;
      d.vy = Math.sin(phi) * Math.sin(theta) * speed * 0.75;
      d.vz = Math.cos(phi) * speed * 0.6 - 2;
      d.maxLife = 1.1 + Math.random() * 1.4;
      d.life = d.maxLife;
      burstPositions[i * 3] = 0;
      burstPositions[i * 3 + 1] = 0;
      burstPositions[i * 3 + 2] = -2;
      const c = Math.random() < 0.6 ? burstBase : burstBase2;
      burstColors[i * 3] = c.r;
      burstColors[i * 3 + 1] = c.g;
      burstColors[i * 3 + 2] = c.b;
    }
    burstGeo.attributes.position.needsUpdate = true;
    burstGeo.attributes.color.needsUpdate = true;
    burstPoints.visible = true;
  }

  // ==================== 阶段状态与过渡 ====================
  let phase: WishPhase = 'idle';
  let phaseStart = 0; // performance.now() 基准
  let pendingRarityCurrent = pendingRarity;
  const flashSprite = new THREE.Sprite(
    track(new THREE.SpriteMaterial({ map: cometCoreTex, color: new THREE.Color('#ffffff'), transparent: true, opacity: 0, depthWrite: false, blending: THREE.AdditiveBlending }))
  );
  flashSprite.position.z = 2;
  scene.add(flashSprite);

  const clock = new THREE.Clock();
  let rafId = 0;
  let disposed = false;
  // 流星飞行路径: 深空左上远端 → 掠向镜头中心 (3D 二次贝塞尔, 透视近大远小)
  const cometFrom = new THREE.Vector3(-38, 26, -58);
  const cometCtrl = new THREE.Vector3(-8, 12, -30);
  const cometTo = new THREE.Vector3(0, 0, 2);
  const cometPos = new THREE.Vector3();
  const cometDir = new THREE.Vector3();
  const Y_AXIS = new THREE.Vector3(0, 1, 0);
  let cometEase = 0;
  let shakeIntensity = 0;
  // 阶段过渡缓动: 每个可淡入元素维护目标强度, 每帧向目标靠近
  const fade = { rays: 0, raysTarget: 0, pillar: 0, pillarTarget: 0, ringCycle: 0 };

  function setPhase(next: WishPhase) {
    phase = next;
    phaseStart = performance.now();

    cometGroup.visible = next === 'meteor';
    flashSprite.material.opacity = next === 'flash' ? 1 : 0;

    if (next === 'meteor') {
      bloom.strength = 0.55;
      cometMat.uniforms.uOpacity.value = 1;
    }
    if (next === 'flash') {
      bloom.strength = 2.4;
      shakeIntensity = 0.5;
      // 闪光瞬间: 尾迹粒子再补一波
      emitTrailBurst();
    }
    if (next === 'reveal') {
      applyTheme(pendingRarityCurrent); // 揭晓瞬间"炸色": 中性蓝白 → 真实稀有度主题
      bloom.strength = 0.8;
      raysGroup.visible = true;
      pillar.visible = true;
      fade.raysTarget = 0.6;
      fade.pillarTarget = 0.65;
      detonateBurst();
    }
    if (next === 'result') {
      bloom.strength = 0.75;
      fade.raysTarget = 0.32; // 保留缓慢氛围旋转
      fade.pillarTarget = 0;
      pillar.visible = true;
      rings.forEach(r => {
        r.visible = false;
      });
    }
  }

  /** flash 瞬间在原点补一波尾迹粒子 */
  function emitTrailBurst() {
    for (let i = 0; i < trailCount / 2; i += 1) {
      const d = trailData[trailCursor];
      trailCursor = (trailCursor + 1) % trailCount;
      d.life = 0.6 + Math.random() * 0.5;
      d.vx = (Math.random() - 0.5) * 10;
      d.vy = (Math.random() - 0.5) * 10;
      d.vz = (Math.random() - 0.5) * 6;
      const pos = trailGeo.attributes.position.array as Float32Array;
      pos[trailCursor * 3] = 0;
      pos[trailCursor * 3 + 1] = 0;
      pos[trailCursor * 3 + 2] = 0;
      const col = trailGeo.attributes.color.array as Float32Array;
      col[trailCursor * 3] = 1;
      col[trailCursor * 3 + 1] = 0.95;
      col[trailCursor * 3 + 2] = 0.8;
    }
    trailGeo.attributes.position.needsUpdate = true;
    trailGeo.attributes.color.needsUpdate = true;
  }

  /** 流星贝塞尔轨迹 (0→1 进度) */
  function cometAt(t: number, out: THREE.Vector3) {
    const u = 1 - t;
    out.set(0, 0, 0);
    out.addScaledVector(cometFrom, u * u);
    out.addScaledVector(cometCtrl, 2 * u * t);
    out.addScaledVector(cometTo, t * t);
  }

  // ==================== 主循环 ====================
  function resize() {
    const w = canvas.clientWidth || window.innerWidth;
    const h = canvas.clientHeight || window.innerHeight;
    renderer.setSize(w, h, false);
    composer.setSize(w, h);
    bloom.setSize(w, h);
    starMat.uniforms.uPixelRatio.value = renderer.getPixelRatio();
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
  }
  resize();
  window.addEventListener('resize', resize);

  function tick() {
    if (disposed) return;
    rafId = requestAnimationFrame(tick);
    const dt = Math.min(clock.getDelta(), 0.05);
    const t = clock.elapsedTime;
    const pElapsed = (performance.now() - phaseStart) / 1000;

    // ---- 星空穿越: 星星向相机漂移 + 循环回深处, 相机轻微呼吸 ----
    const pos = starGeo.attributes.position.array as Float32Array;
    const drift = phase === 'result' ? 4 : 9;
    for (let i = 0; i < starCount; i += 1) {
      pos[i * 3 + 2] += drift * dt;
      if (pos[i * 3 + 2] > 14) pos[i * 3 + 2] = -130;
    }
    starGeo.attributes.position.needsUpdate = true;
    stars.rotation.z += dt * 0.01;
    starMat.uniforms.uTime.value = t;
    cometMat.uniforms.uTime.value = t;

    // ---- 流星阶段: 贝塞尔飞行 + 尾迹发射 ----
    if (phase === 'meteor' && cometGroup.visible) {
      const progress = Math.min(pElapsed / 2.3, 1);
      cometEase = progress * progress * (3 - 2 * progress); // smoothstep
      cometAt(cometEase, cometPos);
      cometGroup.position.copy(cometPos);
      cometMat.uniforms.uOpacity.value = Math.min(progress * 10, 1);
      // Mesh 轴向: 局部 +Y (尾端) 指向速度反方向, 头部迎向运动
      cometAt(Math.max(cometEase - 0.012, 0), cometDir);
      cometDir.sub(cometPos).normalize();
      cometGroup.quaternion.setFromUnitVectors(Y_AXIS, cometDir.clone().negate());
      cometGlow.position.y = -COMET_LOCAL_HALF;
      // 尾迹粒子发射 (核心后方, 世界坐标)
      if (progress < 1) {
        const tailX = cometPos.x - cometDir.x * COMET_LOCAL_HALF;
        const tailY = cometPos.y - cometDir.y * COMET_LOCAL_HALF;
        const tailZ = cometPos.z - cometDir.z * COMET_LOCAL_HALF;
        for (let k = 0; k < 3; k += 1) {
          const d = trailData[trailCursor];
          trailCursor = (trailCursor + 1) % trailCount;
          d.life = 0.4 + Math.random() * 0.4;
          d.vx = (Math.random() - 0.5) * 2;
          d.vy = (Math.random() - 0.5) * 2;
          d.vz = (Math.random() - 0.5) * 2;
          const pArr = trailGeo.attributes.position.array as Float32Array;
          pArr[trailCursor * 3] = tailX + (Math.random() - 0.5) * 0.7;
          pArr[trailCursor * 3 + 1] = tailY + (Math.random() - 0.5) * 0.7;
          pArr[trailCursor * 3 + 2] = tailZ + (Math.random() - 0.5) * 0.7;
          const cArr = trailGeo.attributes.color.array as Float32Array;
          cArr[trailCursor * 3] = primary.r;
          cArr[trailCursor * 3 + 1] = primary.g;
          cArr[trailCursor * 3 + 2] = primary.b;
        }
        trailGeo.attributes.position.needsUpdate = true;
        trailGeo.attributes.color.needsUpdate = true;
      }
      if (progress >= 1) shakeIntensity = 0.35;
    }

    // ---- 尾迹粒子衰减 (颜色→黑即隐) ----
    {
      const cArr = trailGeo.attributes.color.array as Float32Array;
      const pArr = trailGeo.attributes.position.array as Float32Array;
      let dirty = false;
      for (let i = 0; i < trailCount; i += 1) {
        const d = trailData[i];
        if (d.life <= 0) continue;
        dirty = true;
        d.life -= dt;
        const f = Math.max(d.life, 0);
        cArr[i * 3] *= 0.94;
        cArr[i * 3 + 1] *= 0.94;
        cArr[i * 3 + 2] *= 0.94;
        pArr[i * 3] += d.vx * dt;
        pArr[i * 3 + 1] += d.vy * dt;
        pArr[i * 3 + 2] += d.vz * dt;
        if (d.life <= 0) {
          cArr[i * 3] = 0;
          cArr[i * 3 + 1] = 0;
          cArr[i * 3 + 2] = 0;
        }
      }
      if (dirty) {
        trailGeo.attributes.color.needsUpdate = true;
        trailGeo.attributes.position.needsUpdate = true;
      }
    }

    // ---- 闪光阶段: 白核膨胀 ----
    if (phase === 'flash') {
      const p = Math.min(pElapsed / 0.45, 1);
      flashSprite.scale.setScalar(4 + p * 28);
      flashSprite.material.opacity = 1 - p;
      cometGroup.visible = false;
      bloom.strength = 2.4 * (1 - p) + 0.9;
    }

    // ---- reveal/result: 光线旋转 + 光柱 + 冲击环 + 爆发粒子 ----
    fade.rays += (fade.raysTarget - fade.rays) * Math.min(dt * 3.5, 1);
    fade.pillar += (fade.pillarTarget - fade.pillar) * Math.min(dt * 3, 1);
    raysMatA.uniforms.uTime.value = t;
    raysMatB.uniforms.uTime.value = t;
    raysMatA.uniforms.uIntensity.value = fade.rays;
    raysMatB.uniforms.uIntensity.value = fade.rays * 0.8;
    const raysScale = phase === 'reveal' ? Math.min(0.3 + pElapsed * 1.2, 1) : 1;
    raysGroup.scale.setScalar(raysScale);
    pillarMat.uniforms.uTime.value = t;
    pillarMat.uniforms.uIntensity.value = fade.pillar;
    pillar.scale.y = phase === 'reveal' ? Math.min(0.25 + pElapsed * 1.6, 1) : 1;

    if (phase === 'reveal') {
      // 三道冲击波循环扩散
      rings.forEach((ring, i) => {
        ring.visible = true;
        const cycle = ((pElapsed + i * 0.45) % 1.35) / 1.35;
        ring.scale.setScalar(0.5 + cycle * 22);
        ringMats[i].opacity = (1 - cycle) * 0.85;
        ring.rotation.z += dt * (i % 2 === 0 ? 0.4 : -0.3);
      });
    }

    if (burstPoints.visible) {
      const pArr = burstGeo.attributes.position.array as Float32Array;
      const cArr = burstGeo.attributes.color.array as Float32Array;
      let alive = false;
      for (let i = 0; i < burstCount; i += 1) {
        const d = burstData[i];
        if (d.life <= 0) continue;
        alive = true;
        d.life -= dt;
        const damp = Math.pow(0.35, dt); // 指数衰减速度
        d.vx *= damp;
        d.vy *= damp;
        d.vz *= damp;
        d.vy -= 1.2 * dt; // 轻微下坠
        pArr[i * 3] += d.vx * dt;
        pArr[i * 3 + 1] += d.vy * dt;
        pArr[i * 3 + 2] += d.vz * dt;
        const f = Math.max(d.life / d.maxLife, 0);
        cArr[i * 3] = burstBase.r * f;
        cArr[i * 3 + 1] = burstBase.g * f;
        cArr[i * 3 + 2] = burstBase.b * f;
      }
      if (alive) {
        burstGeo.attributes.position.needsUpdate = true;
        burstGeo.attributes.color.needsUpdate = true;
      } else {
        burstPoints.visible = false;
      }
    }

    // ---- 相机编排: 流星随行推进 → 撞击急推 → 揭示后拉, 叠加呼吸与震动 ----
    let targetZ = 27;
    if (phase === 'meteor') targetZ = 27 - cometEase * 6.5;
    else if (phase === 'flash') targetZ = 19;
    camera.position.z += (targetZ - camera.position.z) * Math.min(dt * (phase === 'flash' ? 7 : 2), 1);
    camera.position.x = Math.sin(t * 0.4) * 0.3 + (shakeIntensity > 0 ? (Math.random() - 0.5) * shakeIntensity : 0);
    camera.position.y = Math.cos(t * 0.33) * 0.2 + (shakeIntensity > 0 ? (Math.random() - 0.5) * shakeIntensity : 0);
    shakeIntensity = Math.max(0, shakeIntensity - dt * 1.2);
    camera.lookAt(0, 0, -4);

    composer.render();
  }
  rafId = requestAnimationFrame(tick);

  // ==================== 对外 API ====================
  /** 把真实稀有度主题刷到揭示阶段发光材质上 (流星恒为中性蓝白, 不参与换色) */
  function applyTheme(r: number) {
    const th = rarityTheme(r);
    primary.set(th.primary);
    secondary.set(th.secondary);
    bgColor.set('#070b1e').lerp(primary, 0.07);
    scene.background = bgColor;
    // 天空照片整体染上稀有度色 (炸色的一环); 压得较暗, 防止亮色照片+光线+bloom 叠加过曝
    skyMat.color.copy(new THREE.Color(th.primary)).lerp(new THREE.Color('#0a0e24'), 0.82);
    raysMatA.uniforms.uColor.value = primary.clone();
    raysMatB.uniforms.uColor.value = secondary.clone().lerp(new THREE.Color('#ffffff'), 0.3);
    pillarMat.uniforms.uColor.value = primary.clone();
    burstBase.set(th.primary);
    burstBase2.set(th.secondary);
    ringMats.forEach((m, i) => m.color.set(i % 2 === 0 ? th.primary : th.secondary));
  }

  return {
    setPhase,
    setRarity(next: number) {
      if (next === pendingRarityCurrent) return;
      pendingRarityCurrent = next;
      // 揭晓前只暂存不换色 (防止流星/闪光阶段靠颜色剧透); reveal 进入时由 setPhase 炸色
      if (phase === 'reveal' || phase === 'result') applyTheme(next);
    },
    dispose() {
      disposed = true;
      cancelAnimationFrame(rafId);
      window.removeEventListener('resize', resize);
      scene.traverse(obj => {
        const mesh = obj as THREE.Mesh;
        if (mesh.geometry) mesh.geometry.dispose();
      });
      disposables.forEach(d => d.dispose());
      composer.dispose();
      renderer.dispose();
    }
  };
}
