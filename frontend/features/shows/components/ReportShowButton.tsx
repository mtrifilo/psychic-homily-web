'use client'

import { useState } from 'react'
import { Flag, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import { useMyShowReport } from '../hooks/useShowReports'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useAuthGatedAction } from '@/lib/hooks/common/useAuthGatedAction'
import { ReportShowDialog } from './ReportShowDialog'
import { LoginPromptDialog } from '@/features/auth'

interface ReportShowButtonProps {
  showId: number
  showTitle: string
  /**
   * `bracket` renders a `<BracketLink>` for the provenance footer's dense
   * `[Report issue]` affordance; the Button variants are the pre-existing
   * action-cluster rendering.
   */
  variant?: 'default' | 'ghost' | 'outline' | 'bracket'
  size?: 'sm' | 'default' | 'lg'
  /** Forwarded to the bracket variant only (sizing inside dense lines). */
  className?: string
}

export function ReportShowButton({
  showId,
  showTitle,
  variant = 'outline',
  size = 'sm',
  className,
}: ReportShowButtonProps) {
  const { isAuthenticated, authStatus } = useAuthContext()
  const { data: myReport, isLoading } = useMyShowReport(
    isAuthenticated ? showId : null
  )
  const [isReportDialogOpen, setIsReportDialogOpen] = useState(false)
  const [isLoginPromptOpen, setIsLoginPromptOpen] = useState(false)
  // Captured when the prompt opens rather than derived during render: the
  // canonical destination reads `window.location.search`, which is empty on
  // the server, and this dialog is mounted (closed) in the server markup.
  const [loginPromptAuthHref, setLoginPromptAuthHref] = useState<string | null>(
    null
  )

  // PSY-476: `myReport?.report !== null` is true when the query is still
  // loading (`myReport` undefined → `undefined !== null` → true), which
  // flashed the disabled "Reported" state before real data arrived. Gate
  // on `!isLoading` and use loose `!= null` so both `undefined` and `null`
  // mean "no existing report".
  const hasReported = !isLoading && myReport?.report != null

  // The sign-in affordance here is a dialog rather than a navigation, so the
  // hook hands over the href instead of pushing it. The pending bail is the
  // part that matters: `!isAuthenticated` reads true for a signed-in viewer
  // whose profile has not arrived, and offering them a sign-in dialog is the
  // same misread the redirect makes elsewhere.
  const { onClick: handleClick } = useAuthGatedAction(
    () => setIsReportDialogOpen(true),
    authHref => {
      setLoginPromptAuthHref(authHref)
      setIsLoginPromptOpen(true)
    }
  )

  // If user has already reported, show a disabled "Reported" affordance
  if (isAuthenticated && hasReported) {
    return variant === 'bracket' ? (
      <BracketLink
        label="Reported"
        disabled
        title="You have already reported this show"
        className={className}
      />
    ) : (
      <Button
        variant="outline"
        size={size}
        disabled
        className="text-muted-foreground"
        title="You have already reported this show"
      >
        <Check className="h-4 w-4 mr-2" />
        Reported
      </Button>
    )
  }

  return (
    <>
      {variant === 'bracket' ? (
        <BracketLink
          label="Report issue"
          onClick={handleClick}
          disabled={isLoading || authStatus === 'pending'}
          title="Report an issue with this show"
          className={className}
        />
      ) : (
        <Button
          variant={variant}
          size={size}
          onClick={handleClick}
          disabled={isLoading || authStatus === 'pending'}
          title="Report an issue with this show"
        >
          <Flag className="h-4 w-4 mr-2" />
          Report Issue
        </Button>
      )}

      {isAuthenticated && (
        <ReportShowDialog
          showId={showId}
          showTitle={showTitle}
          open={isReportDialogOpen}
          onOpenChange={setIsReportDialogOpen}
        />
      )}

      {!isAuthenticated && loginPromptAuthHref && (
        <LoginPromptDialog
          open={isLoginPromptOpen}
          onOpenChange={setIsLoginPromptOpen}
          title="Sign in to report"
          description="You need to be signed in to report an issue with this show. This helps us prevent abuse and keep our community safe."
          authHref={loginPromptAuthHref}
        />
      )}
    </>
  )
}
