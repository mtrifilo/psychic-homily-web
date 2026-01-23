'use client'

import { useState } from 'react'
import { Download, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { API_ENDPOINTS } from '@/lib/api'

interface ExportShowButtonProps {
  showId: number
  showTitle?: string
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'default' | 'sm' | 'lg' | 'icon'
  className?: string
  iconOnly?: boolean
}

/**
 * Export show button component - only visible in development environment
 * Downloads the show as a markdown file
 */
export function ExportShowButton({
  showId,
  showTitle,
  variant = 'ghost',
  size = 'sm',
  className,
  iconOnly = false,
}: ExportShowButtonProps) {
  const [isExporting, setIsExporting] = useState(false)

  // Only render in development
  if (process.env.NODE_ENV !== 'development') {
    return null
  }

  const handleExport = async () => {
    setIsExporting(true)
    try {
      const response = await fetch(API_ENDPOINTS.SHOWS.EXPORT(showId), {
        credentials: 'include',
      })

      if (!response.ok) {
        const errorText = await response.text()
        console.error('Export failed:', response.status, errorText)
        if (response.status === 404) {
          alert('Export not available. Make sure ENVIRONMENT=development is set on the backend.')
        } else {
          alert(`Export failed: ${response.status} ${response.statusText}`)
        }
        return
      }

      // Get filename from Content-Disposition header or generate one
      const contentDisposition = response.headers.get('Content-Disposition')
      let filename = `show-${showId}.md`
      if (contentDisposition) {
        const match = contentDisposition.match(/filename="?([^"]+)"?/)
        if (match) {
          filename = match[1]
        }
      }

      // Download the file
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      window.URL.revokeObjectURL(url)
      document.body.removeChild(a)
    } catch (error) {
      console.error('Export failed:', error)
      alert('Export failed. Check browser console for details.')
    } finally {
      setIsExporting(false)
    }
  }

  return (
    <Button
      variant={variant}
      size={size}
      onClick={handleExport}
      disabled={isExporting}
      title={`Export ${showTitle || 'show'} as markdown`}
      className={className}
    >
      {isExporting ? (
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
      ) : (
        <Download className="h-3.5 w-3.5" />
      )}
      {!iconOnly && size !== 'icon' && <span className="ml-1">Export</span>}
    </Button>
  )
}
