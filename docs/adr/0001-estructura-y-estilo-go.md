# 0001. Estructura del proyecto y estilo de Go: claridad antes que idioma

- **Estado:** Propuesta
- **Fecha:** 2026-11-05

## Contexto

flores es un proyecto educativo: lo van a leer personas que quieren entender
cómo funciona un coding agent. La pregunta fue si debíamos escribir Go
"idiomático" al estilo de [crush](https://github.com/charmbracelet/crush).

Lo que observamos en crush:

- Unos **50 paquetes** en `internal/` (TUI, SQLite, pubsub, LSP, OAuth,
  servidor HTTP…). Es la estructura de un producto, no de un proyecto para
  aprender: lo esencial (el bucle, el proveedor, las tools) queda diluido.
- **No tiene abstracción de proveedor propia.** La delega en
  [`charm.land/fantasy`](https://github.com/charmbracelet/fantasy), una
  librería parecida al AI SDK de Vercel. Copiar crush ahí significaría "usar una
  librería", justo lo que queremos aprender a construir.

El Go que existe hoy (`benchmarks/tools/mockprovider`, `benchmarks/tools/flores-stub`)
son herramientas del banco de pruebas en `package main` con ficheros planos. Es
deliberado y **no** es el modelo para el agente.

## Decisión

1. **Regla general: el idioma de Go donde aporta claridad, y desviarse a
   propósito (y dejarlo escrito) donde la quite.** En Go casi nunca chocan:
   interfaces pequeñas, paquetes por responsabilidad y poca abstracción *son*
   claridad.
2. **Pocos paquetes, cada uno con una responsabilidad que se pueda decir en una
   frase.** Estructura de partida:

   ```
   cmd/flores/main.go      solo cableado: config → provider → agente
   internal/
     provider/             interfaz, tipos, errores, WithRetry, Collect (ADR 0002)
       anthropic/          adaptador Messages API
       openai/             adaptador Chat Completions (OpenAI y compatibles)
     agent/                el bucle: turnos, límite de pasos, cuándo parar
     tool/                 interfaz Tool + registro
       shell/ read/ edit/  una tool por paquete
     trajectory/           emisor trajectory/v0 (contrato de benchmarks/)
     config/
   benchmarks/             módulo Go aparte (ya existe)
   ```

3. **Cada paquete tiene un `doc.go`** que dice para qué existe y **qué no debe
   hacer** (como la tabla de [`benchmarks/docs/pipeline.md`](../../benchmarks/docs/pipeline.md)).
4. **Un `ARCHITECTURE.md` en la raíz** con el recorrido de una petición de
   principio a fin, cuando exista el agente.
5. **Desviaciones conscientes del idioma** (se irán añadiendo aquí):
   - La interfaz `Provider` vive en su propio paquete (`internal/provider`), no
     en el consumidor (`agent`). El idioma dice "define la interfaz donde se
     usa", pero quien lea el proyecto la buscará en `provider/`.
6. **Sin paquetes cajón de sastre** (`util`, `common`, `helpers`) ni jerarquías
   profundas.

## Por qué

- El objetivo es que otro ingeniero entienda el proyecto real leyendo la
  estructura de carpetas.
- Menos paquetes = menos dependencias internas que seguir = recorrido de una
  petición legible de arriba abajo.

## Alternativas descartadas

- **Copiar la estructura de crush.** Demasiada superficie para un proyecto
  educativo, y esconde el proveedor tras una librería externa.
- **Todo en un paquete.** Legible al principio, pero mezcla responsabilidades en
  cuanto hay más de una tool y más de un proveedor.

## Cómo se valida

- Una persona que no ha visto el código debería poder decir dónde está el bucle,
  dónde se habla con el modelo y dónde se ejecuta una tool solo con `ls internal/`.
- `go vet` y `gofmt` en CI; el resto se revisa en los PR contra este ADR.
