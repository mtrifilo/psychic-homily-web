import { ShowForm } from '@/ShowForm'
import { AiForm } from '@/AiForm'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { AuthProvider, useAuthContext } from '@/lib/context/AuthContext'
import Login from '@/login/Login.tsx'
import Signup from '@/login/Signup.tsx'
import Logout from '@/login/Logout.tsx'

function AppContent() {
    const { isAuthenticated, user, isLoading } = useAuthContext()

    if (isLoading) {
        return (
            <div className="flex justify-center items-center h-64">
                <Card className="w-96">
                    <CardContent className="pt-6">
                        <div className="flex items-center justify-center">
                            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900"></div>
                            <span className="ml-2">Loading...</span>
                        </div>
                    </CardContent>
                </Card>
            </div>
        )
    }

    if (!isAuthenticated) {
        return (
            <div className="flex flex-col items-center justify-center w-full px-4 md:px-16 lg:px-16 lg:pl-16">
                <Card className="w-full max-w-md">
                    <CardHeader className="text-center">
                        <CardTitle className="text-3xl font-bold">Psychic Homily</CardTitle>
                        <CardDescription>
                            Welcome to the music community. Sign in or create an account to continue.
                        </CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-4">
                        <div className="flex gap-4">
                            <Login />
                            <Signup />
                        </div>
                    </CardContent>
                </Card>
            </div>
        )
    }

    return (
        <div className="flex flex-col items-start justify-center w-full max-w-full px-4 md:px-16 lg:px-16 lg:pl-16">
            <Alert className="mb-6 w-full max-w-full">
                <AlertDescription className="flex justify-between items-center flex-wrap gap-2">
                    <div className="flex items-center gap-2">
                        <Badge variant="secondary">Authenticated</Badge>
                        <span className="truncate">Welcome back, {user?.first_name || user?.email}!</span>
                    </div>
                    <Logout />
                </AlertDescription>
            </Alert>

            <Card className="w-full min-w-0">
                <CardHeader>
                    <CardTitle>Show Management</CardTitle>
                    <CardDescription>Submit and manage your music show information</CardDescription>
                </CardHeader>
                <CardContent>
                    <Tabs defaultValue="manualForm" className="w-full">
                        <TabsList className="grid w-full grid-cols-2">
                            <TabsTrigger value="manualForm">Manual Form</TabsTrigger>
                            <TabsTrigger value="AiForm">AI Form</TabsTrigger>
                        </TabsList>
                        <TabsContent value="manualForm" className="mt-6">
                            <ShowForm />
                        </TabsContent>
                        <TabsContent value="AiForm" className="mt-6">
                            <AiForm />
                        </TabsContent>
                    </Tabs>
                </CardContent>
            </Card>
        </div>
    )
}

function App() {
    return (
        <AuthProvider>
            <AppContent />
        </AuthProvider>
    )
}

export default App
