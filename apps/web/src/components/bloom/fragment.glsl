#version 300 es
precision mediump float;

uniform float uFalloff; // 1.5 in pmndrs, 1 in Maxime's
in float vDistance;
out vec4 fragColor;

// A round sprite whose opacity falls with the distance to the focus plane:
// the references' "depth of field" trick.
void main() {
  vec2 cxy = 2.0 * gl_PointCoord - 1.0;
  if (dot(cxy, cxy) > 1.0) discard;
  fragColor = vec4(vec3(1.0), 1.04 - clamp(vDistance * uFalloff, 0.0, 1.0));
}
