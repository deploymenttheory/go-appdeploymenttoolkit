#Requires -Version 7
<#
.SYNOPSIS
Extracts the embedded default config and localized string tables from
PSAppDeployToolkit's ImportsLast.ps1 and emits them as JSON documents for
embedding into the Go toolkit (YAML parsers accept JSON, and these files are
generated artifacts that are never hand-edited).

.EXAMPLE
pwsh ./convert.ps1 -ImportsLastPath /path/to/ImportsLast.ps1 -OutputDirectory ../../internal
#>
[CmdletBinding()]
param
(
    [Parameter(Mandatory = $true)]
    [System.String]$ImportsLastPath,

    [Parameter(Mandatory = $true)]
    [System.String]$OutputDirectory
)

$ErrorActionPreference = 'Stop'

$tokens = $null; $errors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($ImportsLastPath, [ref]$tokens, [ref]$errors)
if ($errors.Count -gt 0)
{
    throw "Parse errors in ${ImportsLastPath}: $($errors[0].Message)"
}

# Find every KeyValuePair[String, ScriptBlock]::new(<key>, { <hashtable> }) in
# document order. The first Empty-keyed pair is the Config block; the second
# Empty-keyed pair and all culture-keyed pairs belong to the Strings table.
$pairs = $ast.FindAll({
        param($node)
        $node -is [System.Management.Automation.Language.InvokeMemberExpressionAst] -and
        $node.Member.Value -eq 'new' -and
        $node.Expression -is [System.Management.Automation.Language.TypeExpressionAst] -and
        $node.Expression.TypeName.FullName -match 'KeyValuePair\[System\.String,\s*System\.Management\.Automation\.ScriptBlock\]' -and
        $node.Arguments.Count -eq 2 -and
        $node.Arguments[1] -is [System.Management.Automation.Language.ScriptBlockExpressionAst]
    }, $true)

if (!$pairs)
{
    throw "No embedded data blocks found in $ImportsLastPath"
}

function Get-PairKey
{
    param($pair)
    $keyArg = $pair.Arguments[0]
    if ($keyArg -is [System.Management.Automation.Language.StringConstantExpressionAst])
    {
        return $keyArg.Value
    }
    return '' # [System.String]::Empty
}

$emptySeen = 0
$outputs = [ordered]@{}
foreach ($pair in $pairs)
{
    $key = Get-PairKey $pair
    if (!$key)
    {
        $emptySeen++
        $key = if ($emptySeen -eq 1) { '__config' } else { 'default' }
    }
    # The scriptblocks are pure data (hashtables of constants); evaluate them.
    $data = & ([scriptblock]::Create($pair.Arguments[1].ScriptBlock.EndBlock.Extent.Text))
    $outputs[$key] = $data
}

$null = New-Item -ItemType Directory -Force -Path "$OutputDirectory/config/embedded", "$OutputDirectory/strtab/embedded"
foreach ($entry in $outputs.GetEnumerator())
{
    $path = if ($entry.Key -eq '__config')
    {
        "$OutputDirectory/config/embedded/config.default.json"
    }
    else
    {
        "$OutputDirectory/strtab/embedded/strings.$($entry.Key).json"
    }
    $entry.Value | ConvertTo-Json -Depth 20 | Set-Content -Path $path -Encoding utf8NoBOM
    Write-Host "Wrote $path"
}
