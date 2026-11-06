#version 300 es
//! Noise: (c) 2011 Ashima Arts, Stefan Gustavson (MIT). Curl: tiagosr/glsl-curl-noise2 (MIT). Simulation: pmndrs/examples (MIT).
// Full notices: THIRD_PARTY_NOTICES.md. Comments are stripped at build time
// except the //! line above.
precision highp float;

uniform mat4 uProjection;
uniform mat4 uView;
uniform float uTime;
uniform float uCurlFreq;
uniform float uFocus;
uniform float uBlur;
uniform float uPixelRatio; // render DPR; point sizes are normalised to the reference's DPR 2
uniform int uVisible;      // particles drawn at full size (1/fov); the rest are 1-pixel dust
uniform float uMaxPoint;   // point size cap, in DPR-2 pixels (same on every device)
uniform int uDetail;       // curl octaves on top of the base one: 0..4 (4 = pmndrs, 3 = Maxime)
uniform bool uCubeSeeds;   // seeds in a cube of side 4 (Maxime) instead of a sphere of radius 128 (pmndrs)
uniform float uBloom;      // 0 = the reference cloud, 1 = petals; values between morph
uniform float uPetals;     // number of petals
uniform float uLayers;     // 1..3 rings of petals
uniform float uTurbulence; // how much of the reference curl motion is added to the petals
uniform float uCup;        // how much the petals curve towards the camera
uniform float uWidth;      // petal width: 1 = neighbouring petals touch
uniform vec2 uOffset;      // screen-space shift, in half screens (-1..1)

out float vDistance;

// ---------------------------------------------------------------------------
// Ashima Arts / Stefan Gustavson webgl-noise (MIT): simplex "snoise", classic "cnoise".
// https://github.com/ashima/webgl-noise
// ---------------------------------------------------------------------------

vec3 mod289(vec3 x) { return x - floor(x * (1.0 / 289.0)) * 289.0; }
vec4 mod289(vec4 x) { return x - floor(x * (1.0 / 289.0)) * 289.0; }
vec4 permute(vec4 x) { return mod289(((x * 34.0) + 1.0) * x); }
vec4 taylorInvSqrt(vec4 r) { return 1.79284291400159 - 0.85373472095314 * r; }
vec3 fade(vec3 t) { return t * t * t * (t * (t * 6.0 - 15.0) + 10.0); }

float snoise(vec3 v) {
  const vec2 C = vec2(1.0 / 6.0, 1.0 / 3.0);
  const vec4 D = vec4(0.0, 0.5, 1.0, 2.0);

  // First corner
  vec3 i = floor(v + dot(v, C.yyy));
  vec3 x0 = v - i + dot(i, C.xxx);

  // Other corners
  vec3 g = step(x0.yzx, x0.xyz);
  vec3 l = 1.0 - g;
  vec3 i1 = min(g.xyz, l.zxy);
  vec3 i2 = max(g.xyz, l.zxy);
  vec3 x1 = x0 - i1 + C.xxx;
  vec3 x2 = x0 - i2 + C.yyy;
  vec3 x3 = x0 - D.yyy;

  // Permutations
  i = mod289(i);
  vec4 p = permute(permute(permute(
             i.z + vec4(0.0, i1.z, i2.z, 1.0))
           + i.y + vec4(0.0, i1.y, i2.y, 1.0))
           + i.x + vec4(0.0, i1.x, i2.x, 1.0));

  // Gradients: 7x7 points over a square, mapped onto an octahedron.
  float n_ = 0.142857142857; // 1.0/7.0
  vec3 ns = n_ * D.wyz - D.xzx;
  vec4 j = p - 49.0 * floor(p * ns.z * ns.z);
  vec4 x_ = floor(j * ns.z);
  vec4 y_ = floor(j - 7.0 * x_);
  vec4 x = x_ * ns.x + ns.yyyy;
  vec4 y = y_ * ns.x + ns.yyyy;
  vec4 h = 1.0 - abs(x) - abs(y);
  vec4 b0 = vec4(x.xy, y.xy);
  vec4 b1 = vec4(x.zw, y.zw);
  vec4 s0 = floor(b0) * 2.0 + 1.0;
  vec4 s1 = floor(b1) * 2.0 + 1.0;
  vec4 sh = -step(h, vec4(0.0));
  vec4 a0 = b0.xzyw + s0.xzyw * sh.xxyy;
  vec4 a1 = b1.xzyw + s1.xzyw * sh.zzww;
  vec3 p0 = vec3(a0.xy, h.x);
  vec3 p1 = vec3(a0.zw, h.y);
  vec3 p2 = vec3(a1.xy, h.z);
  vec3 p3 = vec3(a1.zw, h.w);

  // Normalise gradients
  vec4 norm = taylorInvSqrt(vec4(dot(p0, p0), dot(p1, p1), dot(p2, p2), dot(p3, p3)));
  p0 *= norm.x;
  p1 *= norm.y;
  p2 *= norm.z;
  p3 *= norm.w;

  // Mix final noise value
  vec4 m = max(0.6 - vec4(dot(x0, x0), dot(x1, x1), dot(x2, x2), dot(x3, x3)), 0.0);
  m = m * m;
  return 42.0 * dot(m * m, vec4(dot(p0, x0), dot(p1, x1), dot(p2, x2), dot(p3, x3)));
}

float cnoise(vec3 P) {
  vec3 Pi0 = floor(P);
  vec3 Pi1 = Pi0 + vec3(1.0);
  Pi0 = mod289(Pi0);
  Pi1 = mod289(Pi1);
  vec3 Pf0 = fract(P);
  vec3 Pf1 = Pf0 - vec3(1.0);
  vec4 ix = vec4(Pi0.x, Pi1.x, Pi0.x, Pi1.x);
  vec4 iy = vec4(Pi0.yy, Pi1.yy);
  vec4 iz0 = Pi0.zzzz;
  vec4 iz1 = Pi1.zzzz;

  vec4 ixy = permute(permute(ix) + iy);
  vec4 ixy0 = permute(ixy + iz0);
  vec4 ixy1 = permute(ixy + iz1);

  vec4 gx0 = ixy0 * (1.0 / 7.0);
  vec4 gy0 = fract(floor(gx0) * (1.0 / 7.0)) - 0.5;
  gx0 = fract(gx0);
  vec4 gz0 = vec4(0.5) - abs(gx0) - abs(gy0);
  vec4 sz0 = step(gz0, vec4(0.0));
  gx0 -= sz0 * (step(0.0, gx0) - 0.5);
  gy0 -= sz0 * (step(0.0, gy0) - 0.5);

  vec4 gx1 = ixy1 * (1.0 / 7.0);
  vec4 gy1 = fract(floor(gx1) * (1.0 / 7.0)) - 0.5;
  gx1 = fract(gx1);
  vec4 gz1 = vec4(0.5) - abs(gx1) - abs(gy1);
  vec4 sz1 = step(gz1, vec4(0.0));
  gx1 -= sz1 * (step(0.0, gx1) - 0.5);
  gy1 -= sz1 * (step(0.0, gy1) - 0.5);

  vec3 g000 = vec3(gx0.x, gy0.x, gz0.x);
  vec3 g100 = vec3(gx0.y, gy0.y, gz0.y);
  vec3 g010 = vec3(gx0.z, gy0.z, gz0.z);
  vec3 g110 = vec3(gx0.w, gy0.w, gz0.w);
  vec3 g001 = vec3(gx1.x, gy1.x, gz1.x);
  vec3 g101 = vec3(gx1.y, gy1.y, gz1.y);
  vec3 g011 = vec3(gx1.z, gy1.z, gz1.z);
  vec3 g111 = vec3(gx1.w, gy1.w, gz1.w);

  vec4 norm0 = taylorInvSqrt(vec4(dot(g000, g000), dot(g010, g010), dot(g100, g100), dot(g110, g110)));
  g000 *= norm0.x;
  g010 *= norm0.y;
  g100 *= norm0.z;
  g110 *= norm0.w;
  vec4 norm1 = taylorInvSqrt(vec4(dot(g001, g001), dot(g011, g011), dot(g101, g101), dot(g111, g111)));
  g001 *= norm1.x;
  g011 *= norm1.y;
  g101 *= norm1.z;
  g111 *= norm1.w;

  float n000 = dot(g000, Pf0);
  float n100 = dot(g100, vec3(Pf1.x, Pf0.yz));
  float n010 = dot(g010, vec3(Pf0.x, Pf1.y, Pf0.z));
  float n110 = dot(g110, vec3(Pf1.xy, Pf0.z));
  float n001 = dot(g001, vec3(Pf0.xy, Pf1.z));
  float n101 = dot(g101, vec3(Pf1.x, Pf0.y, Pf1.z));
  float n011 = dot(g011, vec3(Pf0.x, Pf1.yz));
  float n111 = dot(g111, Pf1);

  vec3 fade_xyz = fade(Pf0);
  vec4 n_z = mix(vec4(n000, n100, n010, n110), vec4(n001, n101, n011, n111), fade_xyz.z);
  vec2 n_yz = mix(n_z.xy, n_z.zw, fade_xyz.y);
  return 2.2 * mix(n_yz.x, n_yz.y, fade_xyz.x);
}

// ---------------------------------------------------------------------------
// glsl-curl-noise2 (https://github.com/tiagosr/glsl-curl-noise2), MIT.
// ---------------------------------------------------------------------------

vec3 snoiseVec3(vec3 x) {
  return vec3(
    snoise(x),
    snoise(vec3(x.y - 19.1, x.z + 33.4, x.x + 47.2)),
    snoise(vec3(x.z + 74.2, x.x - 124.5, x.y + 99.4)));
}

vec3 curl(vec3 p) {
  const float e = 0.1;
  vec3 dx = vec3(e, 0.0, 0.0);
  vec3 dy = vec3(0.0, e, 0.0);
  vec3 dz = vec3(0.0, 0.0, e);
  vec3 p_x0 = snoiseVec3(p - dx);
  vec3 p_x1 = snoiseVec3(p + dx);
  vec3 p_y0 = snoiseVec3(p - dy);
  vec3 p_y1 = snoiseVec3(p + dy);
  vec3 p_z0 = snoiseVec3(p - dz);
  vec3 p_z1 = snoiseVec3(p + dz);
  float x = p_y1.z - p_y0.z - p_z1.y + p_z0.y;
  float y = p_z1.x - p_z0.x - p_x1.z + p_x0.z;
  float z = p_x1.y - p_x0.y - p_y1.x + p_y0.x;
  // The reference multiplies by 1 / (2e) before normalising: a no-op, dropped.
  return normalize(vec3(x, y, z));
}

// ---------------------------------------------------------------------------
// Seeds. The reference stores 512² random points on a sphere of radius 128 in
// a texture. Here each particle derives its own from gl_VertexID with an
// integer hash (PCG), using the same distribution (uniform on the sphere).
// ---------------------------------------------------------------------------

uint pcg(uint v) {
  uint s = v * 747796405u + 2891336453u;
  uint w = ((s >> ((s >> 28u) + 4u)) ^ s) * 277803737u;
  return (w >> 22u) ^ w;
}

float unit(uint h) { return float(h) * (1.0 / 4294967296.0); }

// ---------------------------------------------------------------------------
// The reference simulation, applied to one seed instead of one texel. Its
// second "curlPos = curl(curlPos * uCurlFreq + t)" had the same input as "pos",
// so it is reused instead of computed twice (identical result, 1/6 less work).
// ---------------------------------------------------------------------------

vec3 simulate(vec3 seed) {
  float t = uTime * 0.015;
  vec3 pos = curl(seed * uCurlFreq + t);
  vec3 c = pos;
  if (uDetail > 0) c += curl(c * uCurlFreq * 2.0) * 0.5;
  if (uDetail > 1) c += curl(c * uCurlFreq * 4.0) * 0.25;
  if (uDetail > 2) c += curl(c * uCurlFreq * 8.0) * 0.125;
  if (uDetail > 3) c += curl(pos * uCurlFreq * 16.0) * 0.0625;
  return mix(pos, c, cnoise(pos + t));
}

// The cloud, and flower mode: petals are built geometrically (symmetry alone
// does not make petals: the curl output is a nearly uniform shell) and the
// cloud's motion is added on top as organic turbulence. uBloom morphs from one
// to the other. theta picks the petal and the place across it; v the place
// from base to tip.
vec3 bloom(vec3 seed, float theta, float v, float h) {
  vec3 sim = uBloom < 1.0 || uTurbulence > 0.0 ? simulate(seed) : vec3(0.0);
  if (uBloom <= 0.0) return sim;
  const float PI = 3.14159265;
  float layer = floor(h * uLayers);
  float sector = 2.0 * PI / max(uPetals, 1.0);
  float j = floor(theta / sector);
  float u = theta / sector - j - 0.5;              // across the petal, -0.5..0.5
  u = sign(u) * pow(abs(2.0 * u), 0.55) * 0.5;     // denser at the edges: the outline reads
  float w = pow(v, 0.45);                          // base to tip, denser towards the rim
  float profile = pow(sin(PI * pow(w, 0.8)), 0.6); // narrow base, widest past the middle
  float size = 1.05 * (1.0 - 0.3 * layer);
  float angle = (j + 0.5 + 0.5 * layer) * sector + u * sector * uWidth * profile;
  float r = size * w;
  vec3 petal = vec3(r * cos(angle), r * sin(angle), uCup * r * r - 0.12 * layer);
  return mix(sim, petal + sim * uTurbulence * 0.3, uBloom);
}

void main() {
  uint h0 = pcg(uint(gl_VertexID));
  uint h1 = pcg(h0);
  uint h2 = pcg(h1);
  float z = unit(h0) * 2.0 - 1.0;       // uniform z on the sphere (Archimedes)
  float theta = unit(h1) * 6.28318531;  // uniform angle
  float r = sqrt(max(0.0, 1.0 - z * z));
  vec3 seed = uCubeSeeds
    ? (vec3(unit(h0), unit(h1), unit(pcg(h2))) - 0.5) * 4.0 // Maxime: (random - 0.5) * 4 per axis
    : 128.0 * vec3(r * cos(theta), r * sin(theta), z);       // pmndrs: getSphere(…, 128)

  vec4 mvPosition = uView * vec4(bloom(seed, theta, z * 0.5 + 0.5, unit(h2)), 1.0);
  gl_Position = uProjection * mvPosition;
  gl_Position.xy += uOffset * gl_Position.w; // an image shift: the perspective is unchanged
  vDistance = abs(uFocus + mvPosition.z); // mvPosition.z is negative: distance = -z
  gl_PointSize = gl_VertexID < uVisible ? min(vDistance * uBlur, uMaxPoint) * uPixelRatio * 0.5 : 1.0;
}
