# syntax=docker/dockerfile:1
# Habilita a sintaxe atual do Dockerfile.

# Primeira etapa: usamos Go apenas para compilar a aplicação.
# A plataforma AMD64 é a exigida pelo ambiente da Rinha.
FROM --platform=linux/amd64 golang:1.24-alpine AS build

# Diretório de trabalho dentro do container de build.
WORKDIR /src

# Copiamos primeiro os arquivos necessários para a compilação.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

# Os arquivos estáticos são lidos no startup da imagem final.
COPY resources ./resources

# Gera um binário Linux/AMD64 estático e menor.
# CGO desabilitado permite executar o binário em uma imagem scratch.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Segunda etapa: imagem final mínima, sem compilador nem shell.
FROM --platform=linux/amd64 scratch

# Copia somente o executável produzido na etapa anterior.
COPY --from=build /out/api /api

# Copia também o dataset e os metadados usados no startup.
COPY --from=build /src/resources /resources

# Executa como usuário sem privilégios de root.
USER 65532:65532

# Documenta a porta usada pela API dentro do container.
EXPOSE 9999

# Comando executado quando o container iniciar.
ENTRYPOINT ["/api"]
