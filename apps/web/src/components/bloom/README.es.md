# Flores Bloom

> 🇬🇧 [English](README.md)

La "flor" de partículas de la web de flores: un motor WebGL2 sin dependencias.

## De dónde viene

Es un port de la demo *Depth Of Field & Frame Buffer Object*
([r3f.maximeheckel.com/dof-fbo](https://r3f.maximeheckel.com/dof-fbo)), cuyo código
es [`pmndrs/examples` · `gpgpu-curl-noise-dof`](https://github.com/pmndrs/examples/tree/main/examples/gpgpu-curl-noise-dof)
(MIT). El ruido es de Ashima Arts / Stefan Gustavson (MIT) y el curl noise de
[tiagosr/glsl-curl-noise2](https://github.com/tiagosr/glsl-curl-noise2) (MIT).
Avisos completos: [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).

## Dos referencias

La misma demo se publicó dos veces, con diferencias que cambian mucho el aspecto.
Las dos se reproducen con parámetros, como presets en `params.ts`:

| | `pmndrs` (pmndrs/examples) | `maxime` (r3f.maximeheckel.com/dof-fbo) |
|---|---|---|
| Semillas | esfera de radio 128 | cubo de lado 4: remolinos más suaves y grandes |
| Octavas de curl (`detail`) | 4 | 3 |
| Pérdida de opacidad (`falloff`) | 1,5 | 1 |
| Cámara (`lens`, `distance`, `rotate`, `shake`) | 25°, 6, 0,5, 0,2 | 75°, 5, 2, 0 |
| Nombres en su panel | `curl`, `speed`, `aperture` | `frequency`, `speedFactor`, `blur` |

Aquí `blur` es el valor que se usa directamente (el de Maxime); la apertura de
pmndrs se convierte con `blur = (5.6 - aperture) * 9`. En las dos, las partículas
que `fov` "oculta" se siguen dibujando como polvo de 1 píxel (`dust: 1`), porque
la GPU sube su tamaño 0 a 1.

La historia de la landing parte de `maxime` con frequency 0.15, speedFactor 34.5,
fov 0, blur 0.9 y focus 6.90: las 262.144 partículas, pequeñas y nítidas (ver
[Historias](#historias)).

## Cómo funciona, en cuatro ideas

1. **Las partículas parten de una esfera.** Cada una tiene un punto "semilla" fijo.
2. **El curl noise las mueve.** Es un campo suave que parece humo o un fluido. La
   posición de cada partícula depende solo de su semilla y del tiempo.
3. **Desenfoque falso.** Cuanto más lejos está una partícula de la distancia de
   enfoque, más grande y más transparente se dibuja. No hay post-proceso.
4. **Modo flor** (`bloom`, 0–1). Los pétalos se construyen con geometría (cada
   partícula elige un pétalo, un sitio a lo ancho y otro de la base a la punta) y
   encima se suma el movimiento curl del original como turbulencia. Los valores
   entre 0 y 1 transforman la nube en la flor.

## Archivos

| Archivo | Qué es |
|---|---|
| `FloresBloom.astro` | El componente: un canvas, el respaldo y unas líneas que arrancan el motor. |
| `engine.ts` | Configuración de WebGL, bucle de frames, pausas y calidad adaptativa. |
| `vertex.glsl`, `fragment.glsl` | Los shaders. Se minifican al compilar (ver `astro.config.ts`). |
| `camera.ts` | La cámara del original, sin reservar memoria. |
| `params.ts` | Parámetros, valores por defecto y rangos. |
| `scenes.ts` | Historias: escenas ordenadas (juegos completos de parámetros) para páginas con scroll. |
| `story.ts` | El conductor del scroll: convierte la posición de scroll en una escena interpolada. |

## Por qué es ligero

**Menos trabajo que el original.** El original simula 512 × 512 = 262.144
partículas en cada frame en una textura fuera de pantalla (FBO), pero solo dibuja
`1 / fov` de ellas (13.108 con el valor por defecto, `fov = 20`). Su simulación no
tiene memoria: cada frame vuelve a partir de la esfera inicial. Así que aquí las
mismas matemáticas corren en el vertex shader, solo para las partículas que se
dibujan. En la misma máquina, el original iba a 8–13 fps sin límite de frames;
este motor, a cientos.

**Sin buffers.** Cada partícula saca su semilla de su índice (`gl_VertexID`) con
un hash de enteros, con la misma distribución que el original. No se sube nada a
la GPU.

**Nada de memoria nueva en cada frame.** Las matrices, los buffers de tiempos y el
objeto de parámetros del conductor del scroll se reservan una vez. El tamaño solo
se lee cuando cambia el canvas; las anclas de las escenas, cuando cambia la página.

**Render barato.** Canvas opaco (la página es negra de todos modos), sin MSAA (no
aporta nada en sprites redondos), la GPU de bajo consumo preferida y un tope al
tamaño de los puntos para que las partículas muy desenfocadas no cuesten demasiado.

**No bloquea la página.** Los shaders se compilan en paralelo
(`KHR_parallel_shader_compile`) y se comprueban en el primer frame, así que el
título se pinta primero.

**Calidad adaptativa.** El tamaño de los puntos no depende de la densidad de
píxeles de la pantalla, así que el motor puede bajar la resolución sin cambiar el
aspecto: si la mediana de los frames pasa de 18 ms, baja de DPR 2 a 1,5 y luego a 1.
Si sigue lento, el coste está en los vértices (muchas partículas), así que baja
`detail` de octava en octava hasta 1: los remolinos pierden algo de detalle fino y
la forma se mantiene. En pantallas o modos limitados a 30 Hz también se activa, y
es aceptable.

**Pausas.** Se para con la pestaña oculta o con el canvas fuera de pantalla, y con
`prefers-reduced-motion` dibuja un único frame quieto.

## Medidas

Apple M5, Chromium 1248 headless con ANGLE Metal, 1440 × 900 a DPR 2, parámetros
por defecto, build de producción (`astro preview`), 2026-11-05.

| Qué | Resultado |
|---|---|
| Transferencia de la landing | **3 peticiones, 14,1 KB brotli** |
| — HTML con el CSS en línea | 2,4 KB |
| — JS del motor, las escenas y el componente | 5,2 KB |
| — Fuente del título (subconjunto latino) | 6,6 KB |
| Primer pintado con contenido (el título) | 24–56 ms |
| FPS sin límite de frames, valores de la landing (262.144 partículas), 3 ejecuciones, antes de las historias | 318, 322, 330 |
| FPS sin límite de frames, `pmndrs` sin polvo (13.108 partículas), 3 ejecuciones | 867, 869, 882 |
| Memoria JS reservada mientras corre | ~150 B en 5 s (ruido del muestreo) |
| Frames con la pestaña oculta | 0 |
| FPS sin límite de frames en cada escena de la landing (inicio, seda, nube, flor), 3 ejecuciones cada una | 261–271 |
| Memoria JS reservada recorriendo toda la historia con scroll en cada frame | ~2 KB en 1.379 frames |

Sin probar: un móvil real. Con las 262.144 partículas el trabajo está en los
vértices, así que en un móvil lento depende de que la calidad adaptativa baje
`detail`.

Antes de esta pasada, la misma landing pesaba 26,5 KB en 6 peticiones. Los 1,9 MB
que se ven con `bun run dev` son del servidor de desarrollo (módulos sin empaquetar
y la barra de herramientas de Astro); nunca llegan a producción.

`bun run size` (en `apps/web`, después de `bun run build`) imprime la tabla por
archivo y falla si se pasa de **16 KB brotli o 4 peticiones**.

## Uso

```astro
---
import FloresBloom from "../components/bloom/FloresBloom.astro";
---
<FloresBloom class="absolute inset-0" params={{ bloom: 1, rotate: 0, shake: 0, focus: 6 }} />
```

El componente no se posiciona solo: quien lo usa le da el tamaño (arriba,
`absolute inset-0`). Sin WebGL2, o si fallan los shaders, muestra un brillo
estático suave.

## Parámetros

Ver `params.ts`, donde cada uno está documentado. Grupos:

- **Simulación y desenfoque:** `focus`, `speed`, `blur`, `fov` (la fracción de
  partículas que se dibuja, no un ángulo de cámara; 0 = todas), `curl`, `detail`,
  `seeds`, `falloff`, `dust`.
- **Cámara:** `lens` (el campo de visión de verdad), `distance`, `rotate`, `shake`,
  `x` e `y` (posición en
  pantalla, en medias pantallas; en pantallas verticales `x` se reduce y vale 0 por
  debajo de 3:5, para que la forma no se salga de la pantalla de un móvil).
- **Modo flor:** `bloom`, `petals`, `layers`, `turbulence`, `cup`, `width`, `spin`.

Todos los parámetros numéricos se acercan al valor de `set()` con una constante de
tiempo de 0,16 s, igual a 60 que a 120 Hz, y los relojes se integran
(`tiempo += dt · speed`), así que `speed` y `rotate` pueden cambiar sin saltos.

Consejo: con `rotate: 0` y `shake: 0` la cámara mira la flor de frente a
`distance` unidades, así que pon `focus` cerca de `distance` o casi toda saldrá
borrosa.

## Historias

Una historia es una lista de escenas en `scenes.ts`; la página marca un elemento
por escena con `data-bloom-scene="<nombre>"`:

```astro
<FloresBloom class="fixed inset-0" story="landing" />
<section data-bloom-scene="inicio" class="h-dvh">…</section>
<section data-bloom-scene="seda" class="h-dvh">…</section>
```

Cuando el elemento de una escena llega arriba del todo de la ventana, esa escena
está completa; entre dos elementos los parámetros numéricos se interpolan
(smoothstep) y los demás cambian a mitad de camino. `curl` se interpola de forma
geométrica: se percibe como una proporción, así que la forma se dibuja de manera
uniforme en lugar de convertirse en nube enseguida.

La historia de la landing: **inicio** (la forma de seda elegida en la web de
Maxime, en el centro bajo el título) → **seda** a la izquierda → **nube** a la
derecha (`curl: 1`) → **flor** en el centro (`bloom: 1`, más cerca, enfocada y casi
quieta). Siempre está en movimiento (`speed: 34.5`); las posiciones laterales dejan
hueco al contenido. Con
`prefers-reduced-motion` el scroll sigue cambiando la escena, un frame quieto cada
vez.

## Pendiente

Anotado, sin construir aún:

- Clics sobre la flor quieta: un pulso, una onda que empuja las partículas, abrir y
  cerrar.
- Parallax con el puntero, o un puntero que deforma el campo.
- Transiciones disparadas por eventos, no solo por scroll.
- Un canvas que persista entre páginas (`transition:persist`) cuando haya más
  páginas.

## Página de ajuste

`bun run dev:web` y abre `/dev/bloom`: un selector de preset y un control por
parámetro, con FPS y tiempo de CPU y GPU por frame. La URL guarda el preset y los
valores que cambian respecto a él (`?preset=maxime&p={"curl":0.15,"fov":0}`), y
**Copiar parámetros** los copia en JSON. **escena** carga una escena de la historia
de la landing, **progreso** la recorre como lo haría el scroll y **Copiar escena**
copia los valores actuales como una entrada de `scenes.ts`. Añade `&adaptive=0`
para mantener la resolución completa mientras mides. Esta página solo existe en el
servidor de desarrollo.
