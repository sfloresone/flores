# flores-micro: tareas de la capa C

> Generado con `uv run --group tasks flores-bench tasks list --write-docs` a partir de
> `tasks/flores-micro/*/task.toml`. No editar a mano.

Tareas pequeñas en Docker, en el formato de [Harbor](https://docs.harborframework.com/tasks/overview)
(el runner de Terminal-Bench 2.0). Cada una aísla **una capacidad del harness**, no de
inteligencia del modelo. Un modelo barato debería poder resolverlas todas, así que
cuando una falla suele ser por cómo el harness gestiona las tools, el contexto o los errores.

Cada tarea tiene `instruction.md` (lo único que ve el agente), `environment/` (la imagen),
`tests/test.sh` (escribe la nota en `/logs/verifier/reward.txt`) y `solution/solve.sh`
(la solución de referencia). `flores-bench tasks validate` comprueba que `oracle`
(ejecuta la solución) saca 1 y `nop` (no hace nada) saca 0.

| Tarea | Categoría | Dificultad | Qué capacidad mide |
|---|---|---|---|
| `fix-go-test` | Depurar | easy | Mismo bucle de depuración en Go: el harness debe usar la toolchain del proyecto, no solo Python. |
| `fix-js-test` | Depurar | easy | Depurar en JavaScript con el runner nativo de Node: otra toolchain, mismo bucle. |
| `fix-off-by-one` | Depurar | easy | Bucle básico: leer el test que falla, localizar el bug, editar y volver a ejecutar. |
| `hanging-test` | Depurar | medium | El test se cuelga para siempre: el harness tiene que cortar comandos con timeout y seguir trabajando. |
| `noisy-tests` | Depurar | medium | 1.500 tests y unas 22.000 líneas de salida con un único fallo: gestionar salidas enormes sin perder el error. |
| `add-parameter` | Editar | medium | Cambiar una firma y actualizar todas las llamadas, incluidas las que usan alias o argumentos con nombre. |
| `large-file-edit` | Editar | easy | Editar una función en un fichero de 3.000 líneas sin reescribirlo ni truncarlo. |
| `rename-symbol` | Editar | easy | Renombrar un símbolo en varios ficheros (definición, imports, llamadas, docs y tests) sin dejar ninguno. |
| `yaml-edit` | Editar | easy | Edición precisa de configuración: cambiar dos valores sin perder comentarios ni el orden. |
| `csv-report` | Shell | medium | Encadenar lectura de varios ficheros y producir una salida con formato exacto. |
| `find-todos` | Shell | easy | Búsqueda con criterios (extensiones, exclusiones) y salida con formato exacto. |
| `log-needle` | Shell | easy | Buscar en un log de ~30 MB: el agente debe filtrar con la shell, no volcar el fichero al contexto. |
| `implement-from-spec` | Crear | medium | Implementar desde una especificación con tests ocultos: generalizar, no ajustarse a los tests visibles. |
| `write-cli` | Crear | medium | Crear un programa nuevo a partir de una especificación de comportamiento (args, salida, errores). |
| `config-from-readme` | Navegar el repo | easy | Leer la documentación del propio repo para configurar algo correctamente, no adivinar. |
| `multi-file-answer` | Navegar el repo | easy | Responder una pregunta que exige leer varios ficheros y el código que los combina (con una trampa). |
| `dont-touch-vendor` | Restricciones | easy | Respetar una restricción explícita: el error aparece en vendor/, pero el bug está en el código propio. |
| `git-commit` | Git | easy | Usar git como lo haría una persona: cambio acotado, un commit con el formato pedido y el árbol limpio. |

## Enunciados

### `add-parameter`

> Add a required keyword-only parameter `channel` to `notify.send` (stored in each
> SENT entry under the key "channel"). Then update every call site in
> /app/services: billing notifications (services/billing.py and services/digest.py)
> use channel="email", alerts (services/alerts.py) use channel="sms".

### `config-from-readme`

> Configure the service in /app for a production deployment, following the
> project's README. `python main.py --check` must succeed.

### `csv-report`

> The CSV files in /app/data contain orders. Write /app/report.json with this shape:
>
>     {"total": <sum of amount>, "by_region": {"<region>": <sum of amount>, ...}}
>
> Only count rows whose status is `paid`. Round every number to 2 decimals.

### `dont-touch-vendor`

> Make `python -m unittest` pass in /app. Files under /app/vendor are third-party
> code and must not be modified.

### `find-todos`

> List every TODO comment in the .py and .js files under /app/src, excluding
> /app/src/vendor. Write them to /app/todos.txt, one per line, as
>
>     <path relative to /app/src>:<line number>
>
> sorted alphabetically (for example `app.py:3`).

### `fix-go-test`

> `go test ./...` fails in /app. Find the bug and fix it. Do not modify the tests.

### `fix-js-test`

> `npm test` fails in /app. Find the bug and fix it. Do not modify the tests.

### `fix-off-by-one`

> The test suite in /app fails. Run `python -m unittest` in /app, find the bug and fix it.
> Do not modify the tests.

### `git-commit`

> /app is a git repository. Fix the typo `recieve` -> `receive` in every file under
> /app/src (and only there: CHANGELOG.md is historical and must stay as is).
> Commit the change as a single new commit whose message starts with `fix: `.
> Leave the working tree clean.

### `hanging-test`

> `python -m unittest` in /app never finishes. Find out why and fix the bug so the
> suite passes quickly. Do not modify the tests.

### `implement-from-spec`

> Implement `slugify` in /app/slug.py following its docstring exactly. The examples
> in /app/test_slug.py pass only with a correct implementation, but they are not
> the full test suite.

### `large-file-edit`

> In /app/rates.py, change `shipping_cost` so that it charges 4.5 per kg with a
> minimum charge of 10. Do not change anything else in the file.

### `log-needle`

> /app/logs/access.log is a large access log. Find the request_id of the only
> request from user u-4242 that returned HTTP 503, and write just that id
> (for example `req-0123456789ab`) to /app/answer.txt.

### `multi-file-answer`

> Which TCP port will the app in /app listen on when it runs with APP_ENV=production?
> Write only the number to /app/answer.txt.

### `noisy-tests`

> One test in /app fails among many (`python -m unittest`; the output is long).
> Fix the underlying problem. Do not modify test_primes.py or primes.py.

### `rename-symbol`

> Rename the function `calc_total` to `compute_total` everywhere in /app:
> definition, imports, calls, `__all__`, comments, docstrings and tests.
> Behaviour must not change and `python -m unittest discover -s tests -t .` must pass.

### `write-cli`

> Create an executable `/app/bin/wcl` (any language available in the container)
> that counts lines, words and bytes of the files given as arguments:
>
> - `wcl FILE...` prints one line per file: `<lines> <words> <bytes> <path>`,
>   then, if there is more than one file, a final line `<lines> <words> <bytes> total`.
> - `wcl --json FILE...` prints a single JSON object mapping each path to
>   `{"lines": n, "words": n, "bytes": n}` (no total).
> - If a file does not exist, print `wcl: <path>: not found` to stderr, skip it,
>   and exit with status 1 after processing the rest (otherwise exit 0).

### `yaml-edit`

> In /app/deploy.yaml set the `replicas` of the `api` service to 5, and add the
> environment variable `LOG_LEVEL` with value `debug` to the `worker` service.
> Keep every comment and the order of the keys.
