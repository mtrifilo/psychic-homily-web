import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
            <ShowSubmission />
        </QueryClientProvider>
    </StrictMode>
)

// do not lint the following comments
// // Mount login form if element exists
// const loginElement = document.getElementById('login-form')
// if (loginElement) {
//   ReactDOM.createRoot(loginElement).render(
//     <React.StrictMode>
//       <QueryClientProvider client={queryClient}>
//         <Login />
//       </QueryClientProvider>
//     </React.StrictMode>
//   )
// }

// do not lint the following comments
// // Mount user profile if element exists
// const userProfileElement = document.getElementById('user-profile')
// if (userProfileElement) {
//   ReactDOM.createRoot(userProfileElement).render(
//     <React.StrictMode>
//       <QueryClientProvider client={queryClient}>
//         <UserProfile />
//       </QueryClientProvider>
//     </React.StrictMode>
//   )
// }
