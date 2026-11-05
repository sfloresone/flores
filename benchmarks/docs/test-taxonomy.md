# Clasificación de pruebas

> Referencia técnica. Palabras raras: [glosario](glossary.md).

Clasificamos las pruebas por una pregunta: **¿da siempre el mismo resultado?**

De eso depende lo importante: si una prueba puede bloquear un cambio en la CI,
si necesita estadística y si cuesta dinero.

## Los cuatro niveles

| Nivel | Nombre | ¿Cambia entre ejecuciones? | Cómo se comprueba |
|---|---|---|---|
| **N0** | Determinista | No | igualdad exacta |
| **N1** | Determinista con guion | No, si el guion y la versión del agente son los mismos | igualdad exacta sobre lo que hizo el agente |
| **N2** | Medición ruidosa | Sí, un poco: depende de la máquina | estadística (mediana e intervalo) |
| **N3** | Modelo real | Sí, mucho: el modelo no responde igual dos veces | repetir k veces y contar aciertos |

**Regla principal:** nunca se juntan en una misma puerta de calidad una prueba
ruidosa (N2, N3) y una determinista (N0, N1). Si se juntan y falla, no sabemos si
se rompió el código o si la máquina iba lenta.

## N0: determinista

Prueban el propio banco de pruebas. No hay agente ni red.

- Estadística (`tests/test_stats.py`).
- Lectura de los datos de hyperfine (`tests/test_hyperfine.py`).
- Informes y presupuestos (`tests/test_report.py`).
- El mock provider (`tools/mockprovider/*_test.go`).
- El stub (`tools/flores-stub/main_test.go`).

Bloquean un cambio en cualquier máquina. Cuestan 0 € y tardan segundos.

## N1: determinista con guion

Un agente corre contra el mock, que sigue un guion. Como el guion no cambia, el
resultado tampoco. Se comprueba **qué hace el harness**, no cuánto tarda.

- **Contrato headless** (`tests/test_contract.py`): la trayectoria es válida y los
  códigos de salida son los acordados.
- **Capa B** (`scenarios/harness/`): cada escenario simula una situación
  (un error 429, una tool que no existe, un stream cortado…) y dice qué debe hacer
  un harness correcto. Lista completa en [harness-scenarios.md](harness-scenarios.md).
- **Datos estáticos de la capa A**: número de peticiones, bytes enviados al modelo,
  tamaño del system prompt, número de tools. No dependen de la velocidad de la
  máquina.

Bloquean un cambio en cualquier máquina. Cuestan 0 €.

Dos matices:

- Es determinista **para una versión concreta del agente**. Las versiones de los
  competidores están fijadas en `suites/perf/agents.toml`. Si la instalada es otra,
  el informe lo marca con ⚠ y ese resultado no debe usarse como puerta.
- **Trinquete (ratchet).** En `budgets.toml`, la lista `must_pass` de la capa B
  solo crece. Cuando flores arregla un escenario, se añade ahí y ya no puede
  volver a fallar.

## N2: medición ruidosa

Tiempo, CPU y memoria. Con la misma entrada, cada ejecución da un número distinto.

- Arranque (`perf-startup`).
- Un "hello world" completo (`perf-hello`): tiempo, tiempo del harness, CPU, memoria.

Cómo se tratan (detalle en la [metodología](methodology.md#rigor-de-medición)):
varias rondas con orden aleatorio, calentamiento previo, mediana y p95 con su
intervalo de confianza, y descarte de ejecuciones inválidas.

- **Solo bloquea en Linux** (`ubuntu-latest`) y con márgenes amplios. En macOS es
  informativo.
- **Cómo comparar:** si los intervalos de dos medidas se solapan, no hay diferencia
  demostrada. "5,3 ms frente a 5,1 ms" no es una conclusión.

## N3: modelo real

La capa C: tareas reales con modelos reales. Todavía no existe.

- Se mide cuántas tareas se resuelven (pass@1), cuántas se resuelven **siempre**
  en k intentos (pass^k), y el coste y el tiempo.
- **Nunca bloquea un cambio.** Se ejecuta a mano o programada, con un límite de
  gasto, con las claves solo en ese trabajo y con las tareas aisladas en Docker.
- Se repite al menos 3 veces, con las mismas tareas y el mismo modelo, y sin la
  caché del gateway.

## Resumen

| Nivel | ¿Bloquea un cambio? | Dónde | Claves de API | Coste |
|---|---|---|---|---|
| N0 | Sí | cualquier máquina | no | 0 € |
| N1 | Sí | cualquier máquina | no | 0 € |
| N2 | Sí, con margen | solo Linux | no | 0 € |
| N3 | No | manual o programado | solo en ese trabajo | de pago |

## Cómo clasificar una prueba nueva

Responde en orden y para en la primera respuesta que sea "sí":

1. ¿Llama a un modelo real? → **N3**.
2. ¿El resultado depende del tiempo, la CPU o la memoria? → **N2**.
3. ¿Ejecuta un agente contra un guion del mock? → **N1**.
4. En otro caso → **N0**.

Si dudas entre dos niveles, elige el más ruidoso. Un falso aviso es peor que una
prueba que no bloquea.

## Lo que no es una prueba

- **Los competidores** (Pi, Claude Code, Codex, fx) no se prueban: se miden y se
  publican como referencia. No tienen presupuestos.
- **Un número de `perf-hello`** no dice nada sobre la calidad de las respuestas.
  El mock siempre contesta lo mismo.
