/**
 * The references' camera without allocations: a perspective camera at
 * `distance` looking at the origin, with drei's OrbitControls auto-rotate and
 * a CameraShake approximation. Matrices are column-major and written into `out`.
 */
export type Mat4 = Float32Array;

export interface CameraRig {
  distance: number;
  /** OrbitControls autoRotateSpeed: angle per second = 2π / 60 · rotate. */
  rotate: number;
  /** CameraShake intensity (the effect scales with intensity²). */
  shake: number;
}

export function perspective(out: Mat4, fovYDeg: number, aspect: number, near: number, far: number): Mat4 {
  const f = 1 / Math.tan((fovYDeg * Math.PI) / 360);
  const nf = 1 / (near - far);
  out.fill(0);
  out[0] = f / aspect;
  out[5] = f;
  out[10] = (far + near) * nf;
  out[11] = -1;
  out[14] = 2 * far * near * nf;
  return out;
}

/** Smooth pseudo-noise in [-1, 1] (sum of incommensurate sines). */
function wobble(t: number, seed: number): number {
  return Math.sin(t + seed) * 0.5 + Math.sin(t * 2.31 + seed * 1.7) * 0.3 + Math.sin(t * 4.73 + seed * 2.9) * 0.2;
}

/**
 * View matrix at elapsed time `t` (seconds): R(pitch, yaw, roll) · lookAt(eye → origin).
 * - orbit: the reference camera (auto-rotate around Y plus a subtle shake).
 * - front: fixed in front of the flower; it spins around the viewing axis.
 */
export function viewMatrix(out: Mat4, t: number, view: "orbit" | "front", spinAngle: number, rig: CameraRig): Mat4 {
  const DISTANCE = rig.distance;
  let theta = 0, pitch = 0, yaw = 0, roll = spinAngle;
  if (view !== "front") {
    theta = -((2 * Math.PI) / 60) * rig.rotate * t;
    // CameraShake: yaw/pitch 0.05 at 1 Hz, roll 0.5 at 0.5 Hz, scaled by intensity².
    const shake = rig.shake * rig.shake;
    yaw = 0.05 * shake * wobble(t, 11);
    pitch = 0.05 * shake * wobble(t, 23);
    roll = 0.5 * shake * wobble(t * 0.5, 37);
  }
  // lookAt from eye = D · (sin θ, 0, cos θ) to the origin (an orbit around Y):
  // camera axes x = (cos θ, 0, -sin θ), y = (0, 1, 0), z = (sin θ, 0, cos θ),
  // translation (0, 0, -D).
  const st = Math.sin(theta), ct = Math.cos(theta);
  // Camera-space rotation R = Rx·Ry·Rz, element rij = row i, column j.
  const cx = Math.cos(pitch), sx = Math.sin(pitch);
  const cy = Math.cos(yaw), sy = Math.sin(yaw);
  const cz = Math.cos(roll), sz = Math.sin(roll);
  const r00 = cy * cz, r01 = -cy * sz, r02 = sy;
  const r10 = cx * sz + sx * sy * cz, r11 = cx * cz - sx * sy * sz, r12 = -sx * cy;
  const r20 = sx * sz - cx * sy * cz, r21 = sx * cz + cx * sy * sz, r22 = cx * cy;
  // out = R · L. Columns of L's 3×3: (ct, 0, st), (0, 1, 0), (-st, 0, ct); column 3: (0, 0, -D).
  out[0] = r00 * ct + r02 * st; out[1] = r10 * ct + r12 * st; out[2] = r20 * ct + r22 * st; out[3] = 0;
  out[4] = r01; out[5] = r11; out[6] = r21; out[7] = 0;
  out[8] = r02 * ct - r00 * st; out[9] = r12 * ct - r10 * st; out[10] = r22 * ct - r20 * st; out[11] = 0;
  out[12] = -r02 * DISTANCE; out[13] = -r12 * DISTANCE; out[14] = -r22 * DISTANCE; out[15] = 1;
  return out;
}
