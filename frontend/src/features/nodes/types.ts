export const phaseOneProtocols = [
  'shadowsocks',
  'hysteria2',
  'tuic',
  'vless_reality',
  'anytls',
] as const

export type Protocol = (typeof phaseOneProtocols)[number]

export type NodeFixture = {
  id: string
  name: string
  protocol: Protocol
  host: string
  listen_port: number
  public_port: number
  enabled: boolean
  running: boolean
}
