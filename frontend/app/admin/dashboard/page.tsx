'use client'

import { AdminDashboard } from '@/app/admin/dashboard/_components/AdminDashboard'

interface DashboardPageProps {
  onNavigate?: (tab: string) => void
}

export default function DashboardPage({ onNavigate }: DashboardPageProps) {
  return <AdminDashboard onNavigate={onNavigate} />
}
