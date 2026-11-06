# Escenarios de la capa B (comportamiento del harness)

> Generado con `uv run flores-bench scenarios --write-docs` a partir de `scenarios/harness/*.json`.
> No editar a mano: edita el JSON del escenario.

Cada escenario es un guion para el mock provider más lo que un harness correcto
debe hacer, comprobado **solo desde fuera** (código de salida, stdout y las
peticiones que ve el mock). Por eso el mismo escenario juzga a flores y a cualquier
competidor. Las expectativas son la especificación de flores: el TDD de comportamiento.

Las tool calls se escriben de forma canónica (`{"shell": "…"}`) y cada adaptador
las traduce a su tool (`Bash` en Claude Code, `bash` en Pi, `shell` en fx). Los
comandos imprimen un marcador calculado (`echo flores-$((40+2))` → `flores-42`) que
solo aparece si el comando se ejecutó de verdad y su salida volvió al modelo.

## Básicos

### `basic-text`: Respuesta de texto simple

**Por qué:** Control de cordura: un prompt, una petición, el texto llega intacto a stdout. Si esto falla, el resto de resultados no es fiable.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 1; stdout contiene «Hello from the mock provider.». <sub>timeout 60 s</sub>

## Tools

### `malformed-tool-args`: Argumentos de tool con JSON roto

**Por qué:** Con streaming o con modelos pequeños, los argumentos a veces llegan como JSON inválido. El agente debe reportarlo al modelo, no caerse.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 2; resultados de tool ≥ 1; stdout contiene «Recovered from bad arguments.». <sub>timeout 60 s · requiere tool de shell</sub>

### `parallel-tools`: Dos tool calls en el mismo turno

**Por qué:** Los modelos piden varias tools a la vez. Un bug típico es ejecutar solo la primera o mezclar los IDs de las respuestas.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 2; resultados de tool ≥ 2; stdout contiene «Both done.»; el modelo recibe «flores-11»; el modelo recibe «flores-22». <sub>timeout 60 s · requiere tool de shell</sub>

### `tool-chain`: Tres turnos de tools encadenados

**Por qué:** El historial crece turno a turno. Comprueba que el agente conserva todas las salidas anteriores y no pierde ni duplica mensajes.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 4; resultados de tool ≥ 3; stdout contiene «Chain complete.»; el modelo recibe «flores-101»; el modelo recibe «flores-102»; el modelo recibe «flores-103». <sub>timeout 60 s · requiere tool de shell</sub>

### `tool-roundtrip`: Una tool call y su resultado vuelve al modelo

**Por qué:** El bucle básico de un agente: ejecutar la tool y devolver su salida real al modelo. El marcador solo existe en la salida del comando (flores-$((40+2)) -> flores-42), así que prueba que el comando se ejecutó de verdad.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 2; resultados de tool ≥ 1; stdout contiene «Done.»; el modelo recibe «flores-42». <sub>timeout 60 s · requiere tool de shell</sub>

### `unknown-tool`: El modelo pide una tool que no existe

**Por qué:** Los modelos alucinan nombres de tools. El agente no debe romperse: tiene que devolver un error como resultado de la tool y dejar que el modelo se corrija.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 2; resultados de tool ≥ 1; stdout contiene «Recovered from unknown tool.». <sub>timeout 60 s</sub>

## Errores del proveedor

### `auth-error-fail-fast`: 401: fallar rápido y sin reintentos

**Por qué:** Una clave inválida no se arregla reintentando. Reintentar con backoff hace esperar al usuario minutos para nada.

**Se espera:** sale con error; termina antes del timeout; peticiones = 1; ≤ 10 s. <sub>timeout 60 s</sub>

### `rate-limit-recover`: 429 transitorio: reintentar respetando Retry-After

**Por qué:** Los proveedores devuelven 429 con frecuencia. El agente debe reintentar (respetando Retry-After) y terminar la tarea, sin mostrar el error al usuario.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 3; reintentos = 2; stdout contiene «Recovered after rate limit.»; ≤ 30 s. <sub>timeout 60 s</sub>

### `retry-after-too-long`: 429 con Retry-After de una hora: fallar en el acto

**Por qué:** Si el proveedor pide esperar una hora, reintentar antes gasta peticiones que van a fallar, y esperar sin avisar deja al usuario colgado. Lo correcto es fallar enseguida, explicando cuánto pidió esperar.

**Se espera:** sale con error; termina antes del timeout; peticiones = 1; ≤ 5 s. <sub>timeout 45 s</sub>

### `server-error-recover`: 503 transitorio: reintentar

**Por qué:** Los errores 5xx y de sobrecarga son transitorios. Un solo fallo no debe tumbar la sesión.

**Se espera:** sale con 0; termina antes del timeout; reintentos = 1; stdout contiene «Recovered after server error.»; ≤ 30 s. <sub>timeout 60 s</sub>

## Streaming

### `huge-output`: Respuesta de 1 MiB

**Por qué:** Salidas enormes (ficheros generados, logs) ponen a prueba el buffering del stream y del render. No debe colgarse ni disparar la memoria. No se limita el número de peticiones: compactar el contexto tras una respuesta enorme (lo que hace Pi) es una decisión legítima.

**Se espera:** sale con 0; termina antes del timeout; stdout contiene «flores-big-0123456789abcdef-flores-big»; ≤ 30 s. <sub>timeout 60 s</sub>

### `slow-first-token`: Primer token a los 8 s

**Por qué:** Los modelos con razonamiento tardan en emitir el primer token. Un timeout demasiado agresivo aborta respuestas válidas.

**Se espera:** sale con 0; termina antes del timeout; peticiones = 1; stdout contiene «Worth the wait.». <sub>timeout 60 s</sub>

### `stream-truncated-recover`: Stream cortado a mitad: reintentar

**Por qué:** Las conexiones SSE se cortan (proxies, redes móviles). El agente debe detectar que el stream no terminó y reintentar, sin dar por buena una respuesta a medias.

**Se espera:** sale con 0; termina antes del timeout; reintentos ≥ 1; stdout contiene «The quick brown fox jumps over the lazy »; ≤ 30 s. <sub>timeout 60 s</sub>

## Límites

### `runaway-tool-loop`: El modelo pide tools sin fin

**Por qué:** Un modelo atascado puede pedir tools para siempre. El agente necesita un límite propio de pasos y debe parar solo (con un error claro) antes de quemar tokens.

**Se espera:** cualquier código de salida; termina antes del timeout; peticiones ≤ 50. <sub>timeout 45 s</sub>
