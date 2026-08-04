import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, Cpu, Database, Gauge, Server, Shield, Zap } from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { getDashboardHealth, getChannelHealth, getCircuitBreakers, reloadConfig } from '../api'

function StatCard({
  title,
  value,
  icon: Icon,
  variant = 'default',
  subtitle,
}: {
  title: string
  value: string | number
  icon: React.ElementType
  variant?: 'default' | 'success' | 'warning' | 'danger'
  subtitle?: string
}) {
  const colorMap = {
    default: 'text-blue-600 dark:text-blue-400',
    success: 'text-green-600 dark:text-green-400',
    warning: 'text-yellow-600 dark:text-yellow-400',
    danger: 'text-red-600 dark:text-red-400',
  }

  return (
    <Card className="relative overflow-hidden">
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        <Icon className={`h-4 w-4 ${colorMap[variant]}`} />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        {subtitle && (
          <p className="text-xs text-muted-foreground mt-1">{subtitle}</p>
        )}
      </CardContent>
    </Card>
  )
}

function ProgressBar({ value, max = 100, label }: { value: number; max?: number; label: string }) {
  const pct = Math.min((value / max) * 100, 100)
  const color = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-green-500'
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs">
        <span>{label}</span>
        <span>{value.toFixed(1)}%</span>
      </div>
      <div className="h-2 rounded-full bg-muted">
        <div className={`h-2 rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

export function PerformanceDashboardPanel() {

  const { data: health, isLoading: healthLoading } = useQuery({
    queryKey: ['dashboard-health'],
    queryFn: getDashboardHealth,
    refetchInterval: 10_000,
  })

  const { data: channelHealth } = useQuery({
    queryKey: ['channel-health'],
    queryFn: getChannelHealth,
    refetchInterval: 30_000,
  })

  const { data: breakers } = useQuery({
    queryKey: ['circuit-breakers'],
    queryFn: getCircuitBreakers,
    refetchInterval: 15_000,
  })

  if (healthLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-32" />
        ))}
      </div>
    )
  }

  const openBreakers = breakers?.filter((b) => b.state === 'open') ?? []
  const unhealthyChannels = channelHealth?.filter((c) => !c.success) ?? []

  return (
    <div className="space-y-6">
      {/* System Status Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="CPU Usage"
          value={health ? `${health.system.cpu_usage.toFixed(1)}%` : 'N/A'}
          icon={Cpu}
          variant={health && health.system.cpu_usage > 80 ? 'danger' : health && health.system.cpu_usage > 50 ? 'warning' : 'success'}
        />
        <StatCard
          title="Memory Usage"
          value={health ? `${health.system.memory_usage.toFixed(1)}%` : 'N/A'}
          icon={Server}
          variant={health && health.system.memory_usage > 80 ? 'danger' : health && health.system.memory_usage > 50 ? 'warning' : 'success'}
        />
        <StatCard
          title="Active Requests"
          value={health?.request_queue.active_requests ?? 0}
          icon={Activity}
          subtitle={health?.request_queue.enabled ? `Queue: ${health.request_queue.queue_size}` : 'Queue disabled'}
        />
        <StatCard
          title="Rate Limit"
          value={health?.adaptive_limit.enabled ? `${health.adaptive_limit.current_limit}/s` : 'Disabled'}
          icon={Gauge}
          variant={health?.adaptive_limit.enabled ? 'default' : 'warning'}
        />
      </div>

      {/* System Resources */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-4 w-4" />
            System Resources
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {health && (
            <>
              <ProgressBar value={health.system.cpu_usage} label="CPU" />
              <ProgressBar value={health.system.memory_usage} label="Memory" />
              <ProgressBar value={health.system.disk_usage} label="Disk" />
            </>
          )}
        </CardContent>
      </Card>

      {/* Channel Health */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span className="flex items-center gap-2">
              <Server className="h-4 w-4" />
              Channel Health
            </span>
            <Badge variant={unhealthyChannels.length > 0 ? 'destructive' : 'secondary'}>
              {unhealthyChannels.length} unhealthy
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {channelHealth && channelHealth.length > 0 ? (
            <div className="space-y-2">
              {channelHealth.slice(0, 10).map((ch) => (
                <div key={ch.channel_id} className="flex items-center justify-between py-1 border-b last:border-0">
                  <div className="flex items-center gap-2">
                    <div className={`h-2 w-2 rounded-full ${ch.success ? 'bg-green-500' : 'bg-red-500'}`} />
                    <span className="text-sm">{ch.channel_name}</span>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground">
                    {ch.success ? (
                      <span>{ch.latency_ms}ms</span>
                    ) : (
                      <span className="text-red-500">{ch.error_msg}</span>
                    )}
                    {ch.consecutive_failures > 0 && (
                      <Badge variant="outline" className="text-xs">
                        {ch.consecutive_failures}x failed
                      </Badge>
                    )}
                  </div>
                </div>
              ))}
              {channelHealth.length > 10 && (
                <p className="text-xs text-muted-foreground text-center pt-2">
                  +{channelHealth.length - 10} more channels
                </p>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No health data available</p>
          )}
        </CardContent>
      </Card>

      {/* Circuit Breakers */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span className="flex items-center gap-2">
              <Shield className="h-4 w-4" />
              Circuit Breakers
            </span>
            <Badge variant={openBreakers.length > 0 ? 'destructive' : 'secondary'}>
              {openBreakers.length} open
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {breakers && breakers.length > 0 ? (
            <div className="space-y-1">
              {breakers.slice(0, 10).map((b) => (
                <div key={b.channel_id} className="flex items-center justify-between py-1 text-sm">
                  <span>Channel #{b.channel_id}</span>
                  <Badge variant={b.state === 'open' ? 'destructive' : b.state === 'half_open' ? 'default' : 'secondary'}>
                    {b.state}
                  </Badge>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">All circuit breakers are closed</p>
          )}
        </CardContent>
      </Card>

      {/* Actions */}
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={() => window.location.reload()}>
          <Zap className="h-4 w-4 mr-1" />
          Refresh
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={async () => {
            await reloadConfig()
            window.location.reload()
          }}
        >
          <AlertTriangle className="h-4 w-4 mr-1" />
          Reload Config
        </Button>
      </div>
    </div>
  )
}