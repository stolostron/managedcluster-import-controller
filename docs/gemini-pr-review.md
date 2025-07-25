# Gemini AI PR Review Setup Guide

This document explains how to set up and use the Gemini AI-powered PR review system for the managedcluster-import-controller project.

## Overview

The Gemini PR Review system automatically reviews pull requests using Google's Gemini AI model, providing intelligent feedback on:

- Code quality and Go best practices
- Security vulnerabilities
- Performance considerations
- Kubernetes and OCM integration patterns
- Testing coverage suggestions
- Documentation requirements

## Setup Instructions

### 1. Prerequisites

- Repository admin access to configure GitHub Secrets
- Google AI Studio account for Gemini API access

### 2. Configure Gemini API Key

1. **Get API Key from Google AI Studio**:
   - Visit [Google AI Studio](https://aistudio.google.com/apikey)
   - Sign in with your Google account
   - Create a new API key
   - Copy the generated API key

2. **Add GitHub Secret**:
   - Go to your repository's **Settings** → **Secrets and variables** → **Actions**
   - Click **New repository secret**
   - Name: `GEMINI_API_KEY`
   - Value: Paste your Gemini API key
   - Click **Add secret**

### 3. Workflow Configuration

The workflow is already configured in `.github/workflows/gemini-pr-review.yml` and will:

- **Automatically trigger** on PR events (opened, synchronized, reopened)
- **Manual trigger** via comment: `@gemini-cli /review`
- **Skip** documentation-only changes and certain paths
- **Require permissions** for manual triggers (OWNER, MEMBER, or COLLABORATOR)

## Usage

### Automatic Reviews

The system automatically reviews PRs when:
- A new PR is opened
- An existing PR is updated with new commits
- A PR is reopened

### Manual Reviews

Users with appropriate permissions can trigger reviews by commenting:
```
@gemini-cli /review
```

### Review Scope

The AI reviewer focuses on:

1. **Code Quality**
   - Go best practices and idioms
   - Error handling patterns
   - Code organization and structure

2. **Security**
   - Potential vulnerabilities
   - Sensitive data exposure
   - Authentication and authorization

3. **Performance**
   - Inefficient operations
   - Resource usage optimization
   - Scalability considerations

4. **Kubernetes Best Practices**
   - Proper API usage
   - Resource management
   - Controller patterns

5. **OCM Integration**
   - Compatibility with OCM components
   - ManifestWork handling
   - Cluster lifecycle management

6. **Testing**
   - Test coverage gaps
   - Missing test scenarios
   - Integration test needs

7. **Documentation**
   - Required documentation updates
   - API documentation
   - Usage examples

## Customization

### Project Context

The AI reviewer uses the `GEMINI.md` file for project-specific context, including:
- Architecture overview
- Coding standards
- Common issues to watch for
- Review focus areas

### Workflow Modifications

To customize the workflow behavior, edit `.github/workflows/gemini-pr-review.yml`:

- **Change trigger conditions**: Modify the `on` section
- **Adjust file filters**: Update `paths-ignore` patterns
- **Modify review prompt**: Edit the `prompt` section
- **Change permissions**: Adjust the `if` condition for manual triggers

## Troubleshooting

### Common Issues

1. **Workflow not triggering**
   - Check if `GEMINI_API_KEY` secret is properly configured
   - Verify the PR doesn't match ignored paths
   - Ensure the workflow file is in the default branch

2. **Permission denied for manual triggers**
   - Only users with OWNER, MEMBER, or COLLABORATOR permissions can trigger manual reviews
   - Check your repository role and permissions

3. **API rate limits**
   - Gemini API has usage limits
   - Consider implementing retry logic or reducing review frequency

### Debugging

Check the GitHub Actions logs:
1. Go to **Actions** tab in your repository
2. Find the "Gemini PR Review" workflow run
3. Click on the failed run to see detailed logs
4. Check the "Run Gemini PR Review" step for error messages

## Best Practices

### For Reviewers
- Use AI feedback as a starting point, not a replacement for human review
- Verify AI suggestions before implementing changes
- Consider the context and project-specific requirements

### For Contributors
- Address AI feedback constructively
- Ask for clarification if suggestions are unclear
- Use manual triggers sparingly to avoid API quota issues

### For Maintainers
- Regularly review and update the `GEMINI.md` configuration
- Monitor API usage and costs
- Adjust workflow triggers based on team needs

## Security Considerations

- The Gemini API key should be kept secure and rotated regularly
- The AI has access to PR content but not to the broader repository
- Review AI suggestions for potential security implications
- Don't rely solely on AI for security reviews

## Limitations

- AI reviews are based on code patterns and may miss context-specific issues
- The system cannot run tests or verify functionality
- Complex architectural decisions may require human expertise
- API rate limits may affect review frequency

## Support

For issues with the Gemini PR Review system:
1. Check the troubleshooting section above
2. Review GitHub Actions logs for error details
3. Consult the [Gemini CLI Action documentation](https://github.com/google-gemini/gemini-cli-action)
4. Open an issue in the repository for persistent problems
