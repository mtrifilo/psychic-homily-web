import { Button } from '@/components/ui/button'
import { useAuth } from '@/lib/context/AuthContext'

function Logout() {
    const { logout, isLoading } = useAuth()

    const handleLogout = () => {
        logout()
    }

    return (
        <Button onClick={handleLogout} disabled={isLoading} variant="outline">
            {isLoading ? 'Logging out...' : 'Logout'}
        </Button>
    )
}

export default Logout
