import type { NodeFixture, Protocol } from './types'

export const mockNodes: NodeFixture[] = [
  {
    id: 'tokyo-edge',
    name: '东京边缘节点',
    protocol: 'vless_reality',
    host: 'tokyo.subbox.example',
    listen_port: 443,
    public_port: 24443,
    enabled: true,
    running: true,
  },
  {
    id: 'singapore-core',
    name: '新加坡核心节点',
    protocol: 'hysteria2',
    host: 'singapore.subbox.example',
    listen_port: 8443,
    public_port: 8443,
    enabled: true,
    running: true,
  },
  {
    id: 'los-angeles-west',
    name: '洛杉矶西部节点',
    protocol: 'anytls',
    host: 'los-angeles.subbox.example',
    listen_port: 443,
    public_port: 443,
    enabled: true,
    running: true,
  },
]

const clientSchemes: Record<Protocol, string> = {
  shadowsocks: 'ss',
  hysteria2: 'hysteria2',
  tuic: 'tuic',
  vless_reality: 'vless',
  anytls: 'anytls',
}

export function getMockClientLink(node: Pick<NodeFixture, 'name' | 'protocol' | 'host' | 'public_port'>) {
  return `${clientSchemes[node.protocol]}://mock-client@${node.host}:${node.public_port}#${encodeURIComponent(node.name)}`
}
