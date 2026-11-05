# 0003. El bucle del agente

> 🇬🇧 [English](en/0003-agent-loop.md)

- **Estado:** Aceptada
- **Fecha:** 2026-11-05

## Contexto

Antes de escribir el bucle de flores revisamos cómo lo hacen cuatro harnesses
reales. Leímos su código, no su documentación (enlaces fijados a un commit):

| | Dónde está el bucle | Límite de pasos por defecto | Otros frenos | Tools en paralelo |
|---|---|---|---|---|
| **fx** (Zig) | [`orchestrator.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/agent/runtime/orchestrator.zig#L7131) | ninguno ([`default_max_agent_steps = 0`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/config/agent_steps.zig#L3)) | — | las de solo lectura, en grupos |
| **opencode** (TS) | [`driveLoop`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/llm.ts#L218) y el [runner de sesión](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/session/runner/llm.ts#L252) | ninguno (`steps` es opcional) | al llegar al límite: un paso sin tools que pide un resumen | todas, sin límite |
| **crush** (Go) | delegado en [fantasy](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/agent.go#L440) | ninguno | [repetición](https://github.com/charmbracelet/crush/blob/c7854d54a8fc2eef37f5c31ee4e829318b1ea13c/internal/agent/loop_detection.go#L12) (más de 5 veces en 10 pasos) y contexto casi lleno | las marcadas `Parallel`, hasta 5 |
| **Pi** (TS) | [`runLoop`](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/agent/src/agent-loop.ts#L172) | ninguno | — | por defecto sí |

Lo que aprendimos:

- **El núcleo es el mismo en los cuatro** y cabe en unas 50 líneas: llamar al
  modelo, ejecutar las tools que pida, añadir los resultados y repetir hasta que
  conteste sin pedir tools. Las diferencias están en los bordes: límites,
  paralelismo, persistencia y recuperación.
- **Ninguno limita los pasos por defecto.** En la capa B lo medimos: en
  `runaway-tool-loop` (el modelo pide tools sin parar) Pi, Claude Code, Codex y
  fx hicieron entre 3.000 y 7.000 peticiones en 45 s, hasta que el runner los
  paró. Con un modelo real, el freno es la persona que mira, el cupo de la
  suscripción o el límite de gasto de la consola del proveedor.

## Decisión

1. **Dos bucles.** El **REPL** (exterior) solo existe en modo interactivo: lee un
   mensaje, ejecuta un turno, imprime y espera. El **bucle del agente**
   (interior) va dentro de cada turno. `flores -p` es una sola vuelta del
   exterior. Los dos modos usan el mismo bucle interior, así que el banco de
   pruebas mide el código que usa la gente.
2. **Vocabulario.** Un **turno** va de un mensaje del usuario a la respuesta
   final. Un **paso** es una llamada al modelo. Un turno tiene uno o más pasos.
   El flag es `--max-steps` y la trayectoria usa `step` (`trajectory/v1`).
3. **El bucle es código propio**, al estilo de fantasy: legible de arriba abajo y
   sin librerías. Vive en `internal/agent` y expone
   `Run(ctx, cfg, prompt) iter.Seq2[Event, error]`, igual que el proveedor
   ([ADR 0002](0002-interfaz-provider.md)).
4. **Límite de pasos: 0 por defecto, es decir, sin límite**, como los cuatro de
   referencia. `--max-steps N` lo activa para quien lo quiera.
5. **Detección de repeticiones, siempre activa.** Es el freno por defecto contra
   bucles infinitos:
   - Cada tool call tiene una firma: un hash FNV-64a de nombre, argumentos y
     resultado.
   - Si una firma aparece **5 veces en los últimos 10 pasos**, el agente para.
     No hace falta que la ventana esté llena, así que un bucle exacto se detecta
     en el paso 5.
   - Es más estricto que crush, que exige más de 5 repeticiones y una ventana
     completa de 10 pasos.
6. **Al parar por un freno, un paso más para resumir.** El agente añade un
   mensaje que explica por qué para y pide resumir lo hecho, lo pendiente y los
   siguientes pasos. Si hay tools declaradas, ese paso se envía con
   `tool_choice: none`; las tool calls que lleguen igualmente se ignoran. Así
   tanto el usuario como el modelo saben dónde se quedó el trabajo. El mensaje
   es parecido al `MAX_STEPS_PROMPT` de opencode.
7. **Códigos de salida** (los mismos que `flores-stub`):

   | Código | Significado |
   |---|---|
   | 0 | éxito |
   | 2 | uso incorrecto |
   | 3 | error del proveedor |
   | 4 | paró un freno (`max_steps` o `repetition`), aunque haya resumen |
   | 130 | cancelado (Ctrl-C) |

## Por qué

- **El límite por defecto en 0** respeta lo que espera quien viene de otros
  harnesses. Con eso, el freno por defecto pasa a ser la detección de
  repeticiones, que no corta tareas largas pero legítimas.
- **La detección de repeticiones** cubre el caso medido en la capa B (la misma
  llamada con el mismo resultado) sin poner un techo artificial a las tareas
  largas.
- **El paso de resumen** cuesta una petición, pero evita terminar a ciegas: el
  usuario sabe qué quedó hecho y el modelo puede retomarlo.

## Alternativas descartadas

- **Un límite fijo por defecto (25 o 40 pasos).** Protege más, pero corta tareas
  reales largas y se aparta de lo que hacen los demás. Queda disponible con
  `--max-steps`.
- **Cortar sin resumen.** Es más barato, pero ni el usuario ni el modelo saben en
  qué estado quedó el trabajo.
- **Usar fantasy o copiar el orquestador de fx.** fantasy esconde el bucle tras
  más de 20 callbacks. El fichero del bucle de fx tiene más de 12.000 líneas
  porque mezcla permisos, subagentes y recuperación. Los dos son demasiado para
  entenderlo.

## Cómo se valida

- La capa B: `runaway-tool-loop` pasa sin límite de pasos (6 peticiones de 50
  posibles), y `basic-text`, `unknown-tool` y `auth-error-fail-fast` siguen
  pasando.
- Tests del paquete `agent` con un proveedor falso: solo texto, tool
  desconocida, `--max-steps` con resumen, repetición con resumen, error del
  proveedor y cancelación.
- La trayectoria `trajectory/v1` valida contra el esquema en todos los casos.
