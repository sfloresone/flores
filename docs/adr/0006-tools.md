# 0006. Las tools: propias, seis, y lecturas en paralelo

- **Estado:** Aceptada
- **Fecha:** 2026-11-08
- **Desarrolla:** el paso "ejecutar las tools" del [ADR 0003](0003-bucle-del-agente.md)

> La versión en inglés se escribirá junto con el resto de traducciones pendientes.

## Contexto

Hasta M1, flores hablaba con dos proveedores pero no podía **hacer** nada: cada
tool call recibía el error `unknown tool`. En la capa B suspendía
`tool-roundtrip`, `tool-chain` y `parallel-tools` (12/15), y las tareas de la
capa C (editar ficheros, arreglar tests) eran imposibles.

Había que decidir cuatro cosas:
1. si usamos tools propias o las "nativas" de cada proveedor;
2. cuáles;
3. cómo se ejecutan las llamadas de un mismo paso;
4. qué pasa con los permisos.

Leímos cómo lo hacen las cuatro referencias, con el código fijado a los mismos
commits que en el [ADR 0005](0005-adaptador-openai.md), más crush
[`c7854d5`](https://github.com/charmbracelet/crush/tree/c7854d54a8fc2eef37f5c31ee4e829318b1ea13c).

### Tools propias o nativas

Anthropic ofrece tools con un nombre y un esquema fijos con los que ha
entrenado a sus modelos (`bash_20250124`, `text_editor_20250728`). OpenAI tiene
las suyas (`local_shell`, `apply_patch`). Parecen más baratas, pero no lo son:

- **Las tools nativas también las ejecuta el harness.** El proveedor solo fija
  el nombre y el esquema. Leer el fichero, aplicar el cambio o lanzar el
  comando lo hace nuestro código igualmente.
- **Atan a un proveedor.** `text_editor` solo existe en la Messages API y
  `apply_patch` solo en la Responses API de OpenAI, que flores no habla.

**Ninguna de las cuatro referencias las usa para programar.** Todas definen sus
tools como function tools con JSON Schema. Lo único que dejan al proveedor son
las tools que se ejecutan en su servidor, como `web_search`. Lo más parecido es
opencode: da a los modelos
[`gpt-`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/tool/plugin/patch.ts#L291)
una tool `patch` con el formato de parche de Codex, pero como function tool
normal.

### Lo que hace cada referencia

| | fx | opencode | crush | Pi |
|---|---|---|---|---|
| Tools por defecto | 6 de código (+ extras) | ~8 | ~25 | **4**: read, bash, edit, write |
| Shell | proceso nuevo por llamada; sesiones para comandos largos | `$SHELL -c`, 120 s por defecto | intérprete `mvdan/sh`, sin timeout | `bash -c`, sin timeout |
| Salida larga del shell | 16 KB, cabeza + cola, fichero aparte | cola, 2000 líneas / 50 KB, fichero | cabeza + cola, 30 K | cola, 2000 líneas / 50 KB, fichero |
| Edit | exacto y único | exacto, luego normaliza comillas y espacios | exacto, luego espacios; [exige leer antes](https://github.com/charmbracelet/crush/blob/c7854d54a8fc2eef37f5c31ee4e829318b1ea13c/internal/agent/tools/edit.go#L282-L284) | exacto, luego fuzzy; varias ediciones por llamada |
| Llamadas de un paso | [lecturas seguidas a la vez](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/agent/runtime/parallel_execution.zig#L19-L30); lo que escribe, solo | todo a la vez | [las marcadas en paralelo, 5 a la vez](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/agent.go#L1670-L1680); bash, edit y write en serie | todo a la vez; escrituras en cola por fichero |
| Respuesta cortada por `max_tokens` | — | — | no ejecuta las llamadas | [no las ejecuta](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/agent/src/agent-loop.ts#L488-L513) y pide repetirlas |
| Permisos en modo headless | revisor con LLM; lo que necesita a una persona falla | rechaza lo que pediría confirmación | aprueba todo | [ninguno](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/coding-agent/docs/security.md#L3) |

fx hace una cosa que copiamos: la descripción de su shell
[pide usar sus tools de ficheros](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/builtins/tools.zig#L73-L76)
en lugar de `ls`, `find`, `grep` o `cat`.

## Decisión

1. **Tools propias, como function tools.** Una tool es un `struct`
   (`internal/tool`), no una interfaz:

   ```go
   type Tool struct {
       Name        string
       Description string
       Schema      json.RawMessage // escrito a mano: los mismos bytes en cada paso
       ReadOnly    bool            // puede ir a la vez que otras lecturas
       Run         func(ctx context.Context, env Env, input json.RawMessage) Result
   }
   ```

   - Cada tool decodifica su entrada en un struct con `DisallowUnknownFields`:
     un campo mal escrito es un error, no un valor que se pierde.
   - Un test comprueba que el esquema escrito a mano y el struct nombran los
     mismos campos.
   - `Run` nunca devuelve un error de Go. Todo lo que el modelo tiene que leer,
     también los fallos, va en el `Result`.
2. **Seis tools: `bash`, `read`, `edit`, `write`, `grep` y `glob`.** Los nombres
   de los campos son los que los modelos más han visto (`command`,
   `old_string`, `new_string`, `replace_all`).
   - **`bash`:**
     - `bash -c` en la carpeta de trabajo, con stdin vacío;
     - 120 s por defecto, 600 como máximo;
     - un grupo de procesos propio: el timeout, Ctrl-C y el final del comando
       paran también a los hijos en segundo plano;
     - una salida de más de 32 KB conserva los primeros 8 KB y los últimos 24 KB,
       y la salida completa queda en un fichero temporal;
     - un código de salida distinto de 0 es un error.
   - **`read`:**
     - `N<TAB>línea`;
     - 2000 líneas o 50 KB por llamada, con `offset` y `limit` para el resto;
     - rechaza binarios y directorios.
   - **`edit`:**
     - reemplazo exacto, único salvo `replace_all`;
     - conserva CRLF, BOM y permisos;
     - cada error dice qué hacer después: releer, o "puede que ya esté hecho".
   - **`write`:** crea los directorios que falten.
   - **`grep` y `glob`:**
     - en Go puro, sin depender de `rg` ni `fd`;
     - ordenados por ruta;
     - saltan `.git` y `node_modules`;
     - con límite y un aviso de cuántos resultados más había.
3. **`tool.Set` concentra los errores comunes:**
   - tool desconocida, con la lista de las que hay;
   - argumentos que no se decodifican, con lo recibido y el esquema esperado;
   - un `panic`;
   - un tope de seguridad de 64 KB por resultado.
4. **Lecturas en paralelo, escrituras en orden.** Las llamadas `ReadOnly`
   seguidas (`read`, `grep`, `glob`) forman un grupo y se ejecutan a la vez.
   `bash`, `edit`, `write` y cualquier tool desconocida van solas. Los
   resultados vuelven siempre **en el orden de las llamadas**.
5. **Una respuesta cortada por `max_tokens` no ejecuta nada.** Sus argumentos
   pueden estar incompletos, así que cada llamada recibe un error que pide
   repetirla con los argumentos completos.
6. **Sin permisos en M2.** `flores -p` ejecuta lo que pida el modelo, como Pi y
   `crush run`, y `--no-tools` lo apaga todo. Los permisos llegarán con la TUI,
   en su propio hito.
7. **El system prompt gana la carpeta de trabajo y el sistema operativo.** Las
   rutas son relativas a esa carpeta, y los comandos dependen del sistema.

## Por qué

- **Agnóstico y medible.** Las mismas seis tools funcionan con Anthropic, con
  OpenAI y con cualquier servidor compatible. El banco mide los dos protocolos
  con la misma lista `must_pass`.
- **El coste en tokens es pequeño, y está medido.** `perf-hello` en macOS,
  bytes enviados al modelo en la primera petición:

  | | Tools | Petición |
  |---|---:|---:|
  | flores (Anthropic) | 6 | 4,2 KiB |
  | flores (OpenAI) | 6 | 4,4 KiB |
  | Pi 1.5.0 | 4 | 5,9 KiB |
  | fx 0.0.16 | 13 | 25,0 KiB |
  | Claude Code 2.1.319 | 12 | 45,2 KiB |

  A partir del segundo paso, la caché de prompt cobra esos bytes a precio
  reducido, porque no cambian. El presupuesto de `request_bytes` pasa de 4096 a
  5120 B para que una descripción que engorde se note.
- **Seis y no cuatro.** Con `grep` y `glob` propias, la salida está acotada y
  ordenada aunque el sistema no tenga `rg`. Además, el modelo no tiene que
  adivinar las opciones de `find` y `grep` de cada sistema. Cuestan 1,6 KB por
  petición (900 B `grep` y 694 B `glob`, de 4010 B en total para las seis).
- **Paralelizar solo las lecturas** da la velocidad donde importa, en varias
  lecturas seguidas, sin carreras: nada lee un fichero mientras otra llamada lo
  escribe, y dos `bash` nunca se pisan.
- **Parar el grupo de procesos** es lo que hace posible la tarea `hanging-test`
  de la capa C: un test que se cuelga no puede retener el turno ni sobrevivir a
  flores.

## Alternativas descartadas

- **Las tools nativas de cada proveedor.** No ahorran código, rompen el ser
  agnósticos y ninguna referencia las usa.
- **Una interfaz `Tool` con métodos**, como fantasy. Un struct con una función
  se lee de arriba abajo y basta mientras ninguna tool tenga estado.
- **Esquemas generados por reflexión**, como fantasy. Los bytes dependerían del
  orden de los campos y de las etiquetas. Escrito a mano, más un test que lo
  compara con el struct, es más corto y estable.
- **Edición fuzzy, multi-edit, `apply_patch` o exigir leer antes de editar.**
  Cada una arregla un fallo que todavía no hemos visto. Se añadirán si la capa C
  muestra ediciones fallidas o a ciegas.
- **Comandos en segundo plano o con sesión**, como fx o crush (`job_output`).
  Hoy el comando y sus hijos terminan con la llamada; un servidor de desarrollo
  que deba seguir vivo es trabajo de otro hito.
- **Ejecutarlo todo en paralelo**, como Pi y opencode. Obliga a serializar las
  escrituras por fichero, y aun así dos `bash` pueden pisarse.

## Riesgo aceptado: sin permisos

Sin permisos, un modelo puede borrar ficheros, leer secretos o ejecutar
cualquier cosa con los permisos del usuario, también fuera de la carpeta de
trabajo. Un fichero o la salida de un comando pueden llevar instrucciones que
lo empujen a hacerlo (prompt injection). Hoy flores solo corre contra el mock y
en los contenedores de la capa C. Para usarlo con un modelo real en una máquina
propia, hazlo dentro de un contenedor o de una carpeta desechable, o con
`--no-tools`.

## Cómo se valida

- **`internal/tool`:** un test por comportamiento de cada tool. Por ejemplo,
  `sleep 30 & sleep 30` con timeout de 1 s termina en 1 s, y `seq 1 100000`
  deja un fichero completo con sus 100 000 líneas. Además:
  - las lecturas de un grupo corren a la vez de verdad: dos tools que se esperan
    entre sí terminan;
  - los esquemas coinciden con los structs.
- **`internal/agent`:**
  - las salidas reales vuelven al modelo en el orden de las llamadas;
  - una respuesta cortada no ejecuta nada.
- **`cmd/flores`:** un servidor falso pide `bash`, y la petición siguiente lleva
  `flores-42`.
- **Capa B: 15/15** con `flores` y con `flores-openai`, los tres escenarios de
  tools incluidos en `must_pass`.
- **Tests de contrato:**
  - la salida de `bash` llega al modelo en los dos protocolos, y nunca antes de
    ejecutarse;
  - las seis tools viajan con los mismos bytes en cada paso;
  - unos argumentos rotos vuelven explicados con el esquema.
