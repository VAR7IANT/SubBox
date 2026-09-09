import { useState } from 'react'
import { AppShell } from './components/layout/AppShell'
import type { NavigationLabel } from './components/layout/Sidebar'
import { NodeCards } from './features/nodes/NodeCards'
import { SubscriptionCard } from './features/subscription/SubscriptionCard'

const pageCopy: Record<NavigationLabel, { title: string; description: string }> = {
  总览: {
    title: '总览',
    description: '管理订阅、节点与 Sing-box 服务。',
  },
  节点: {
    title: '节点',
    description: '查看和管理你的 Sing-box 节点。',
  },
  部署: {
    title: '部署',
    description: '准备一个新的 Sing-box 部署。',
  },
  中转: {
    title: '中转',
    description: '管理 NAT 与端口转发关系。',
  },
  设置: {
    title: '设置',
    description: '配置 SubBox 的基础偏好。',
  },
}

function App() {
  const [activeItem, setActiveItem] = useState<NavigationLabel>('总览')
  const currentPage = pageCopy[activeItem]

  return (
    <AppShell
      activeItem={activeItem}
      onNavigate={setActiveItem}
      pageTitle={currentPage.title}
      pageDescription={currentPage.description}
      nodeContent={<NodeCards layout={activeItem === '总览' ? 'overview' : 'page'} />}
    >
      {activeItem === '总览' ? <SubscriptionCard /> : null}
    </AppShell>
  )
}

export default App
