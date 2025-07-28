import { ShowForm } from '@/ShowForm'
import { AiForm } from '@/AiForm'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import './components/ui/react-app-wrapper.css'

function App() {
    return (
        <div className="react-app-isolated">
            <div className="flex flex-col items-start justify-center w-full px-4 md:px-16 lg:px-16 lg:pl-16">
                <h1 className="w-full mb-4">Submit a Show</h1>
                <Tabs defaultValue="manualForm">
                    <TabsList>
                        <TabsTrigger value="manualForm">Manual Form</TabsTrigger>
                        <TabsTrigger value="AiForm">AI Form</TabsTrigger>
                    </TabsList>
                    <TabsContent value="manualForm">
                        <ShowForm />
                    </TabsContent>
                    <TabsContent value="AiForm">
                        <AiForm />
                    </TabsContent>
                </Tabs>
            </div>
        </div>
    )
}

export default App
