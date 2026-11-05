# Glosario

Las palabras que usa esta documentación, explicadas sin jerga.

## El agente

**Modelo.** El LLM (Claude, GPT…). Recibe texto y contesta con texto. Por sí solo
no puede leer ficheros ni ejecutar nada.

**Coding agent.** Un programa que le da "manos" al modelo para trabajar con
código: leer y editar ficheros, ejecutar comandos, etc.

**Harness.** La parte del agente que *no* es el modelo: el programa que lo rodea.
Se encarga de hablar con el modelo, ejecutar lo que este pida y decidir cuándo
parar. **flores es un harness.** Lo medimos por separado del modelo porque es la
parte que controlamos.

**El bucle (loop).** Lo que hace un agente, en cuatro pasos que se repiten:

1. El agente envía al modelo la conversación y la lista de tools.
2. El modelo contesta con texto o con una petición ("ejecuta este comando").
3. Si pidió una tool, el agente la ejecuta y añade el resultado a la conversación.
4. Vuelve al paso 1. Termina cuando el modelo contesta solo con texto.

**Tool.** Una acción que el modelo puede pedir: ejecutar un comando, leer un
fichero, editar un fichero. El modelo solo *pide*; la ejecuta el harness.

**Paso.** Una vuelta del bucle: una llamada al modelo (en inglés, *step*).

**Turno.** Lo que pasa entre un mensaje del usuario y la respuesta final. Un
turno tiene uno o más pasos: el modelo puede pedir tools varias veces antes de
contestar. Ver [ADR 0003](../../docs/adr/0003-bucle-del-agente.md).

**Contexto.** Todo lo que el modelo ve en cada llamada: instrucciones, historial y
resultados de las tools. Crece con cada paso, y enviarlo cuesta tiempo y dinero.

**System prompt.** Las instrucciones fijas que el harness envía siempre al
principio. Cuanto más largo, más cuesta cada sesión.

**Headless.** Modo sin interfaz: `flores -p "..."` hace la tarea, escribe
el resultado y sale. Es lo que necesitamos para poder medirlo.

## El banco de pruebas

**Benchmark.** Una prueba que produce números para comparar.

**Mock provider.** Un servidor falso que imita a OpenAI o a Anthropic. Contesta
lo que le dice un guion, al instante y gratis. Así probamos el harness sin gastar
dinero ni depender de un modelo real.

**Escenario.** El guion del mock: "primero pide esta tool, luego contesta con este
texto", o "contesta siempre con un error 429".

**Trayectoria.** El registro, evento a evento, de lo que hizo el agente en una
ejecución: peticiones, tools, errores, resultado. Es un fichero con un evento
JSON por línea.

**Sujeto.** Lo que se mide: flores, Pi, Claude Code, Codex, fx. Los que no son
flores son solo referencia para comparar.

**Capas A, B y C.** A mide velocidad y recursos; B mide si el harness se
comporta bien ante situaciones concretas; C mide si resuelve tareas reales con un
modelo real. Ver la [metodología](methodology.md).

## Estadística

**Determinista.** Da siempre el mismo resultado con la misma entrada.

**Ruidoso.** Cambia un poco cada vez aunque la entrada sea igual, por ejemplo el
tiempo que tarda un programa.

**Mediana (p50).** El valor del medio al ordenar las medidas. Aguanta mejor que la
media una medida lenta por casualidad.

**p95.** El valor por debajo del cual está el 95 % de las medidas. Muestra los
casos lentos.

**IC95 (intervalo de confianza).** El rango donde, con un 95 % de confianza, está la
mediana real. Si los rangos de dos agentes se solapan, **no** podemos decir que uno
sea más rápido que el otro.

**Presupuesto (budget).** Un límite que no se puede superar (por ejemplo, "arrancar
en menos de X ms"). Si se supera, la CI falla.

**CI.** Las comprobaciones automáticas que GitHub ejecuta en cada cambio.

**RSS.** La memoria que ocupa el proceso mientras corre.
