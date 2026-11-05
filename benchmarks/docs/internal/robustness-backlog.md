# Notas internas: robustez del banco de pruebas

> **Esto no es documentación de usuario.** Son tareas y dudas nuestras. Cuando una
> se resuelva, se borra de aquí y, si procede, se explica en la referencia técnica.
> Revisión hecha el 2026-11-05, justo después de F2.

Prioridad: 🔴 puede dar resultados engañosos · 🟡 mejora · 🟢 menor.

## Puertas de calidad

- 🔴 **Los límites de tiempo son frágiles en CI.** `budgets.toml` exige
  `wall_ms.p50 ≤ 5.0` al arranque de flores-stub. En el M5 mide 3,1 ms; un runner
  compartido de GitHub puede pasar de 5 ms sin que el código haya cambiado.
  Propuesta: separar los límites **deterministas** (`request_bytes`,
  `system_bytes`, `llm_requests`, `tools`; duros) de los de **tiempo** (anchos, o
  solo avisan hasta tener historial en Linux).
- 🟡 **Los competidores se instalan con `continue-on-error`.** Si falla, el sujeto
  desaparece del informe. El informe ya dice "no instalado", pero la CI debería
  imprimir un resumen de qué se midió y qué no.
- 🟡 **Nadie vigila `history.jsonl`.** Un límite fijo no detecta una subida lenta.
  Propuesta: comparar con la mediana de las últimas N ejecuciones en Linux.

## Tests que faltan en el propio banco

- 🔴 `sanitized_env`: un test que ponga claves falsas en el entorno y compruebe que
  el proceso hijo no las ve. Es la garantía de "no gasta dinero".
- 🔴 Ejecución inválida en `perf-hello`: un agente falso que sale con 0 pero con
  otra salida debe contar como fallo (el caso de fx usando la suscripción real).
- 🟡 `round_orders`: misma semilla → mismo orden; cada sujeto una vez por ronda.
- 🟡 `run_measured`: el timeout mata el proceso, una salida enorme no lo bloquea, y
  el RSS se normaliza entre Linux y macOS.
- 🟡 `budgets.check` con una métrica que falta (debe fallar) y con un sujeto no
  disponible (debe saltarse).
- 🟡 `version_mismatch` y el aviso ⚠ en el informe.
- 🟡 Cada adaptador escribe su config solo dentro del directorio aislado.
- 🟢 Un JSON Schema para `result/v0`, como ya existe para la trayectoria.

## Reproducibilidad

- 🟡 `publish` no avisa si el árbol tenía cambios sin commitear.
- 🟡 El `.deb` de hyperfine no se verifica, y fx se sigue instalando con `curl | bash`
  (sin checksum). ✅ Su versión ya está fijada: la CI pasa `v$(flores-bench pinned --agent fx)`.
- 🟡 Capa C: las imágenes base de las tareas (`python:3.13-slim`, `golang:1.25-bookworm`,
  `node:24-slim`) van por tag, no por digest. Una actualización upstream puede cambiar
  una tarea sin que cambie el repo. La validación oracle/nop lo detectaría solo si rompe.
- 🟢 Las acciones de GitHub están fijadas por tag (`@v4`), no por hash.

## Ruido de medición

- ✅ ~~**RSS de `perf-hello` en Linux tiene un suelo de unos 24 MiB.**~~ Confirmado:
  al hacer `exec`, Linux guarda en el hijo el pico de RSS del espacio de memoria
  anterior (el del padre Python, o el de hyperfine en `perf-startup`).
  Comprobado con `/bin/true` y un padre con 100 MiB de lastre: medía 114 MiB.
  Resuelto con `tools/measure`, que publica su propio suelo (unos 2,5 MiB) y
  marca los valores pegados a él como "≤ suelo". Ver
  [metrics.md](../metrics.md#rss-y-el-suelo-del-lanzador).

- 🟡 Guardar en el resultado lo que afecta al ruido (modo de energía de la CPU,
  carga inicial de la máquina).
- 🟡 Avisar en el informe cuando haya menos de 10 muestras.
- 🟢 Poner también el p95 en los límites, no solo la mediana.

## Orden sugerido

1. Tests de robustez del banco (los 🔴).
2. Separar en `budgets.toml` los límites deterministas de los de tiempo.
3. Partir el job `perf` en "pasada corta en PR" y "referencia en `main`/programada",
   y añadir el resumen "qué se midió" en la CI.
4. Esquema de `result/v0`.
5. Capa C cuando exista el agente.
