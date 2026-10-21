FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/pnp-powershell-mcp-server .

FROM mcr.microsoft.com/powershell:latest
RUN pwsh -NoProfile -Command "Set-PSRepository PSGallery -InstallationPolicy Trusted; Install-Module PnP.PowerShell -Scope AllUsers -Force"
COPY --from=build /out/pnp-powershell-mcp-server /usr/local/bin/pnp-powershell-mcp-server
ENTRYPOINT ["/usr/local/bin/pnp-powershell-mcp-server"]
