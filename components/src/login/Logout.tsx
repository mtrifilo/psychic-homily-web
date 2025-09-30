import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'

function Logout() {
    const { logout, isLoading } = useAuthContext()

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
