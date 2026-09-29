# HyprMod

Editor visual de configuração para o [Hyprland](https://hypr.land/), inspirado no
[nirimod](https://github.com/srinivasr/nirimod), que edita a configuração Lua do
Hyprland **no lugar do arquivo original** em vez de reescrevê-lo.

A ideia central é simples: o HyprMod lê a configuração *executando* ela, entende
o valor efetivo de cada opção e reescreve apenas o trecho exato que mudou.
Comentários, formatação e a lógica que você escreveu continuam intactos.

## Recursos

- **Visão geral** — versão do compositor, provedor de configuração, erros reportados
  pelo Hyprland e o grafo de módulos carregado.
- **Monitores** — canvas com arrastar para reposicionar (sem sobreposição), modos
  disponíveis por saída, escala, VRR e posição.
- **Variáveis** — todas as opções tipadas da API Lua instalada, com controles
  adequados ao tipo (bool, int, float, cor, gradiente, vec2, css_gaps, font_weight).
  Leitura do valor efetivo em runtime via `hyprctl`.
- **Atalhos** — captura de combinação tecla por tecla, menu de dispatchers com
  busca, editor de código Lua, edição in-place de atalhos existentes e detecção de
  conflitos.
- **Regras** — construtor visual de regras de janela, workspace e layer.
- **Animações** — curvas e folhas de animação resolvidas, com pré-visualização.
- **Inicialização e ambiente** — variáveis `hl.env` e callbacks de `hl.on`.
- **Permissões** — regras `hl.permission`.
- **Snapshots** — criar, listar e restaurar pontos de recuperação.
- **Código Lua** — editor CodeMirror com validação, trava otimista e rollback.
- **Interface bilíngue** — inglês e português (BR).

## Como funciona

### Leitura: sandbox Lua real

`internal/sandbox` executa o `hyprland.lua` com o interpretador `lua` do sistema e
uma implementação gravadora da API `hl.*`. Isso é o que permite resolver valores que
um parser não conseguiria: variáveis compartilhadas entre arquivos, expressões
concatenadas, e atalhos gerados por loops. Cada registro sai com arquivo e linha de
origem.

O ambiente é restrito: `os.execute`, escritas em disco e a saída do processo não
estão disponíveis para a configuração avaliada, e há um limite de instruções para
interromper laços infinitos. Callbacks de `hl.on` e `hl.timer` são registrados, mas
nunca executados — igual ao carregamento real do Hyprland.

### Edição: edição cirúrgica

`internal/luasrc` é um lexer e parser Lua escrito em Go puro, sem CGo, que fornece
os spans exatos de byte de cada valor. A escolha evita uma dependência nativa e
torna o comportamento testável; cobre o subconjunto declarativo do qual uma config
é feita.

- **Valor literal** → substitui só o span do valor.
- **Valor computado** (variável, função, loop) → permanece intacto e recebe um bloco
  marcado `-- hyprmod:override <caminho>` inserido no fim do módulo, nunca depois de
  um `return` de nível superior.
- **Atalho existente** → a chamada `hl.bind(...)` é reescrita no lugar.
- **Novo atalho** → bloco marcado `-- hyprmod:add bind ...`.

### Escrita: validar antes de gravar

`internal/writer` executa sempre a mesma sequência:

1. Monta uma cópia temporária do grafo de módulos carregado.
2. Avalia essa cópia no sandbox — a configuração candidata precisa rodar.
3. Valida os tipos das opções contra o schema da API instalada.
4. Cria um snapshot dos arquivos originais.
5. Grava com arquivo temporário e `rename` atômico.
6. Reavalia e, em caso de falha, restaura os arquivos anteriores.

Cada escrita também carrega a trava otimista SHA-256: se o arquivo mudou por fora
desde que a interface o carregou, a gravação é recusada. Qualquer caminho fora de
`~/.config/hypr` é rejeitado.

### Schema: a API instalada como fonte da verdade

`internal/schema` faz o parse do stub `hl.meta.lua` gerado pelo próprio Hyprland
(`/usr/share/hypr/stubs/`), com uma cópia embutida como fallback. As 353 opções
documentadas na versão instalada ficam 100% classificadas em tipos, então a
interface nunca precisa adivinhar um controle.

## Requisitos

- Hyprland **0.55 ou superior** (a partir daí a configuração é Lua; `hyprland.conf`
  legado não é suportado)
- Go 1.26
- Node 24 ou superior
- Um interpretador `lua` no `PATH` (usado para avaliar a configuração)
- `libwebkit2gtk-4.1` e `libgtk-3-dev`

> **Sobre a tag `gtk3`:** o Wails v3 migrou o backend Linux para GTK4/WebKitGTK 6 nas
> betas mais recentes. O HyprMod fixa a `v3.0.0-beta.17`, que ainda suporta o backend
> `gtk3` marcado por tag — é por isso que todo build usa `-tags gtk3`.

## Compilando

```sh
make build        # frontend + binário em bin/hyprmod
```

ou, usando o runner de tarefas do Wails:

```sh
wails3 build
```

Outros alvos:

```sh
make frontend     # só o bundle Vue (npm ci && vue-tsc && vite build)
make bindings     # regenera os bindings TypeScript a partir dos serviços Go
make test         # suíte de testes do core
make test-gui     # suíte incluindo o pacote main com a tag gtk3
```

Depois de `make bindings`, o `main.go` é compilado com a tag `gtk3`.

## Desenvolvimento

```sh
make frontend     # build de produção
cd frontend && npm run dev   # servidor Vite em 127.0.0.1:5173
```

### Testes

A suíte padrão é determinística e não toca na sua configuração. Dois testes
integrados ficam atrás de variáveis de ambiente porque rodam contra a máquina do
desenvolvedor:

```sh
# Lê a configuração real e grava numa cópia temporária — o original não é alterado.
HYPRMOD_TEST_REAL_CONFIG=1 go test ./internal/...

# Consulta a sessão Hyprland em execução (versão, monitores, dispositivos).
HYPRMOD_TEST_LIVE_SESSION=1 go test ./internal/api -run TestLiveHyprlandSession
```

## Estrutura

```
internal/
  config/       modelo de domínio independente de Lua e do Hyprland
  schema/       parse do stub hl.meta.lua em árvore de opções tipadas
  luasrc/       lexer, parser e operações de edição em Lua
  sandbox/      harness Lua que executa a config e registra as chamadas hl.*
  hyprctl/      cliente do socket de controle (eval, getoption, monitors, devices)
  snapshots/    cópias versionadas e restauração
  writer/       pipeline de escrita com validação, snapshot e rollback
  api/          serviços expostos ao frontend via Wails
frontend/       Vue 3 + TypeScript + Vite
packaging/      .desktop e ícone
```

Nenhum pacote interno depende do Wails, o que mantém o core testável e rápido.

## Distribuição

O alvo é Arch e derivados, onde o `webkit2gtk-4.1` já está disponível. O pacote é
compilado com `make build` e distribuído como um `tar.gz` com o binário, o
`.desktop` e o ícone de `packaging/`:

```sh
make build
tar -czf hyprmod-linux-x86_64.tar.gz -C bin hyprmod -C ../packaging .
```

Instalação manual: extraia o binário em `PATH` e copie o `.desktop` e o ícone para
`~/.local/share/applications/` e `~/.local/share/icons/hicolor/scalable/apps/`.

> Não há automação de CI/release neste repositório. Publicar arquivos de workflow no
> GitHub exige uma credencial com o escopo `workflow`, que não estava disponível; os
> testes e o build são executados localmente pelos alvos do `Makefile`.

## Limitações conhecidas

- **Não captura combinações já vinculadas.** O Hyprland consome a tecla antes de
  qualquer janela receber o evento; só layer-shell ou clientes com grab explícito
  CONSEguem, e o Wails não expõe nenhum dos dois. A interface orienta a usar uma
  combinação livre e avisa quando o atalho digitado conflita com um existente.
- **Botões laterais do mouse não são capturados.** O DOM expõe no máximo
  `MouseEvent.button` 0–4 e o WebKitGTK entrega apenas 3 e 4. Os botões entram
  pelos chips de código (`mouse:275` etc.), que usam exatamente a nomenclatura do
  Hyprland.
- **Valores computados não são reescritos no lugar.** Editar um gera um override
  marcado; para mudar a lógica, use o editor Lua.
- **Atalhos com teclas dinâmicas** (gerados por um `for` ou concatenação) são
  marcados como dinâmicos e não podem ser salvos pelo formulário.
- **Mudanças de permissão exigem reinício** do compositor, por segurança do Hyprland.
- **O layout de teclado do heatmap é ANSI fixo**; a detecção de layout por
  dispositivo ainda não foi implementada.
