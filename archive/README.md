# 🗂️ AviAgent Project Archive

This directory contains archived test files, scripts, results, and analysis documents from the AviAgent project development and testing phases.

## 📁 Archive Structure

```
archive/
├── tests/                  # Test scripts and results
│   ├── scripts/           # Test automation scripts
│   ├── results/           # Test output and results
│   └── logs/              # Log files from testing
├── analysis/              # Analysis and documentation
├── misc/                  # Miscellaneous archived files
└── README.md              # This file
```

## 🧪 Tests Directory

### Scripts (`archive/tests/scripts/`)
- **`test_ui_fix.sh`** - Validates UI fixes for tool call display issues
- **`test_mistral_parsing.sh`** - Tests Mistral AI response parsing functionality
- **`test3_mistral_fixed.sh`** - Fixed Mistral AI test with proper JSON formatting
- **`compare_mistral_requests.sh`** - Compares Mistral API requests

### Results (`archive/tests/results/`)
- **`mistral_direct_result.json`** - Results from direct Mistral AI API testing
- **`app_api_result.json`** - Results from AviAgent application API testing
- **`test_results_comparison.md`** - Comprehensive comparison of test results

### Logs (`archive/tests/logs/`)
- **`docker_mistral_logs/`** - Docker container logs from Mistral testing
- **`mistral_comparison_logs/`** - Logs from Mistral API comparison tests

## 📊 Analysis Directory

- **`health_endpoint_analysis.md`** - Detailed analysis of the health endpoint implementation
- **`health_endpoint_fixes.md`** - Documentation of health endpoint fixes and improvements
- **`browser_compatibility_analysis.md`** - Browser compatibility testing and analysis

## 🎯 Miscellaneous Files

- **`cookie-jar`** - Cookie storage file used during testing
- **`backups/`** - Backup files including `start-mistral.sh.backup`

## 📅 Archiving Information

- **Date Archived**: 2024 (exact date can be checked via git history)
- **Purpose**: Clean up project directory while preserving development artifacts
- **Status**: All files are preserved for future reference

## 🔍 How to Use This Archive

1. **Test Scripts**: Can be referenced for understanding test methodologies or reused for regression testing
2. **Test Results**: Provide historical data for comparing current vs past performance
3. **Analysis Documents**: Contain valuable insights about system behavior and fixes
4. **Logs**: Useful for debugging similar issues that may arise in the future

## ⚠️ Important Notes

- These files are archived and should not be modified
- For active development, use the files in the root project directory
- The archive preserves the state of testing as of the archival date
- All essential project files remain in the root directory

## 🔗 Related Files (Not Archived)

The following essential files remain in the root directory:
- `main.go` - Main application entry point
- `config.yaml` - Application configuration
- `start-mistral.sh` and `start-ollama.sh` - Startup scripts
- `Makefile` and `Dockerfile` - Build automation
- `README.md` - Main project documentation