'use client'

import { CalendarCheck, Star } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useRouter } from 'next/navigation'
import { useShowAttendance, useSetAttendance, useRemoveAttendance } from '../hooks/useAttendance'
import type { AttendanceCounts } from '../types'

export interface AttendanceButtonProps {
  showId: number
  /** true for show cards, false for detail page */
  compact?: boolean
  /** Pre-fetched attendance data from batch query (for show lists) */
  attendanceData?: AttendanceCounts
}

export function AttendanceButton({
  showId,
  compact = false,
  attendanceData,
}: AttendanceButtonProps) {
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()

  // Only fetch individual attendance if no batch data provided
  const { data: fetchedData } = useShowAttendance(
    attendanceData ? 0 : showId // disable fetch when batch data exists
  )

  const setAttendance = useSetAttendance()
  const removeAttendance = useRemoveAttendance()

  const data = attendanceData ?? fetchedData
  const goingCount = data?.going_count ?? 0
  const interestedCount = data?.interested_count ?? 0
  const userStatus = data?.user_status ?? ''

  const isGoing = userStatus === 'going'
  const isInterested = userStatus === 'interested'
  const isPending = setAttendance.isPending || removeAttendance.isPending

  const handleClick = (status: 'going' | 'interested') => {
    if (!isAuthenticated) {
      router.push('/auth')
      return
    }

    if (userStatus === status) {
      // Toggle off
      removeAttendance.mutate(showId)
    } else {
      // Set new status
      setAttendance.mutate({ showId, status })
    }
  }

  if (compact) {
    return (
      <TooltipProvider delayDuration={300}>
        <div className="flex items-center gap-0.5">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant={isGoing ? 'default' : 'ghost'}
                size="sm"
                className={cn(
                  'h-7 gap-1 px-1.5 text-xs',
                  isGoing && 'bg-primary text-primary-foreground hover:bg-primary/90',
                  !isGoing && 'text-muted-foreground hover:text-foreground'
                )}
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  handleClick('going')
                }}
                disabled={isPending}
                aria-label={`Going${goingCount > 0 ? ` ${goingCount}` : ''}`}
              >
                <CalendarCheck className="h-3.5 w-3.5" />
                {goingCount > 0 && <span>{goingCount}</span>}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {!isAuthenticated
                ? 'Sign in to RSVP'
                : isGoing
                  ? 'Remove going status'
                  : 'Mark as going'}
            </TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant={isInterested ? 'default' : 'ghost'}
                size="sm"
                className={cn(
                  'h-7 gap-1 px-1.5 text-xs',
                  isInterested && 'bg-primary text-primary-foreground hover:bg-primary/90',
                  !isInterested && 'text-muted-foreground hover:text-foreground'
                )}
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  handleClick('interested')
                }}
                disabled={isPending}
                aria-label={`Interested${interestedCount > 0 ? ` ${interestedCount}` : ''}`}
              >
                <Star className="h-3.5 w-3.5" />
                {interestedCount > 0 && <span>{interestedCount}</span>}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {!isAuthenticated
                ? 'Sign in to RSVP'
                : isInterested
                  ? 'Remove interested status'
                  : 'Mark as interested'}
            </TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    )
  }

  // Full mode (detail page)
  return (
    <div className="flex items-center gap-2">
      <Button
        variant={isGoing ? 'default' : 'outline'}
        size="sm"
        className={cn(
          'gap-2',
          isGoing && 'bg-primary text-primary-foreground hover:bg-primary/90'
        )}
        onClick={() => handleClick('going')}
        disabled={isPending}
      >
        <CalendarCheck className="h-4 w-4" />
        Going
        {goingCount > 0 && (
          <span className={cn(
            'rounded-full px-1.5 py-0.5 text-xs font-medium',
            isGoing
              ? 'bg-primary-foreground/20 text-primary-foreground'
              : 'bg-muted text-muted-foreground'
          )}>
            {goingCount}
          </span>
        )}
      </Button>

      <Button
        variant={isInterested ? 'default' : 'outline'}
        size="sm"
        className={cn(
          'gap-2',
          isInterested && 'bg-primary text-primary-foreground hover:bg-primary/90'
        )}
        onClick={() => handleClick('interested')}
        disabled={isPending}
      >
        <Star className="h-4 w-4" />
        Interested
        {interestedCount > 0 && (
          <span className={cn(
            'rounded-full px-1.5 py-0.5 text-xs font-medium',
            isInterested
              ? 'bg-primary-foreground/20 text-primary-foreground'
              : 'bg-muted text-muted-foreground'
          )}>
            {interestedCount}
          </span>
        )}
      </Button>
    </div>
  )
}
