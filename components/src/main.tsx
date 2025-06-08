import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Theme } from '@radix-ui/themes'
import { ShowSubmission } from './components/ShowSubmission'
import './index.css'

const queryClient = new QueryClient()

const showSubmissionElement = document.getElementById('show-submission')

if (!showSubmissionElement) {
    throw new Error('Could not find show-submission element')
}

const root = createRoot(showSubmissionElement)
root.render(
    <StrictMode>
        <QueryClientProvider client={queryClient}>
            <Theme>
                <ShowSubmission />
            </Theme>
        </QueryClientProvider>
    </StrictMode>
)
