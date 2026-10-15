$ErrorActionPreference = 'Stop'

$branch = (git branch --show-current).Trim()
if (-not $branch) {
	throw 'Could not determine the current Git branch. Check out a branch before running SonarQube analysis.'
}

$line = Get-Content .\sonar.local.env | Where-Object { $_ -match '^SONAR_TOKEN=' } | Select-Object -First 1
$env:SONAR_TOKEN = $line.Substring('SONAR_TOKEN='.Length).Trim().Trim('"')
$env:SONAR_HOST_URL = 'https://sonarcloud.io'

Push-Location frontend
npm ci --ignore-scripts
npm run build
Pop-Location

$env:GOTOOLCHAIN = 'go1.25.0'
go test "-coverprofile=$PWD\coverage.out" ./...

& 'C:\Tools\sonar-scanner\sonar-scanner-8.1.0.6389-windows-x64\bin\sonar-scanner.bat' "-Dsonar.branch.name=$branch"
if ($LASTEXITCODE -ne 0) {
	exit $LASTEXITCODE
}