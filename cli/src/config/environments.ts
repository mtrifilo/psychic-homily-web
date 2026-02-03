export interface Environment {
  name: string;
  key: string;
  apiUrl: string;
}

export const environments: Environment[] = [
  {
    name: 'Local',
    key: 'local',
    apiUrl: 'http://localhost:8080',
  },
  {
    name: 'Stage',
    key: 'stage',
    apiUrl: 'https://stage.api.psychichomily.com',
  },
  {
    name: 'Production',
    key: 'production',
    apiUrl: 'https://api.psychichomily.com',
  },
];

export function getEnvironment(key: string): Environment | undefined {
  return environments.find(env => env.key === key);
}
