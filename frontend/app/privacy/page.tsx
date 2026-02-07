import { Metadata } from 'next'
import Link from 'next/link'

export const metadata: Metadata = {
  title: 'Privacy Policy | Psychic Homily',
  description: 'Privacy Policy for Psychic Homily - how your personal information is collected, used, and protected.',
}

export default function PrivacyPolicyPage() {
  const lastUpdated = 'February 7, 2026'
  const effectiveDate = 'January 31, 2026'

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Privacy Policy</h1>
        <p className="text-center text-muted-foreground mb-8">
          Last Updated: {lastUpdated} | Effective: {effectiveDate}
        </p>

        <div className="prose prose-neutral dark:prose-invert max-w-none space-y-8">
          {/* Introduction */}
          <section>
            <h2 className="text-xl font-semibold mb-3">1. Introduction</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              Psychic Homily is a personal project operated by Matt T. in Phoenix, Arizona. It is not a registered business entity. In this policy, &quot;I,&quot; &quot;me,&quot; and &quot;my&quot; refer to the operator of Psychic Homily.
            </p>
            <p className="text-foreground/90 leading-relaxed mb-3">
              I respect your privacy and am committed to protecting your personal information. This Privacy Policy explains how I collect, use, disclose, and safeguard your information when you use the website and services.
            </p>
            <p className="text-foreground/90 leading-relaxed">
              By using Psychic Homily, you agree to the collection and use of information in accordance with this policy. If you do not agree with these policies and practices, please do not use the services.
            </p>
          </section>

          {/* Information I Collect */}
          <section>
            <h2 className="text-xl font-semibold mb-3">2. Information I Collect</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">2.1 Information You Provide Directly</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Account Information:</strong> Email address, username, password (stored securely as a hash), first name, last name, profile photo/avatar URL, and bio.</li>
              <li><strong>User-Generated Content:</strong> Shows you save to your collection, show submissions, and any other content you create on the platform.</li>
              <li><strong>Uploaded Content:</strong> Images you upload (such as show flyers) for AI-assisted show creation. These images are processed to extract event details and are not stored permanently.</li>
              <li><strong>Preferences:</strong> Theme settings, timezone, language preference, and notification preferences.</li>
              <li><strong>Communications:</strong> Information you provide when contacting me for support or feedback.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">2.2 Information from Third-Party Services</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              If you choose to sign in using OAuth providers, I receive:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Google:</strong> Your Google account ID, email address, name, and profile photo.</li>
              <li><strong>GitHub:</strong> Your GitHub account ID, email address, username, and avatar.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">2.3 Automatically Collected Information</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Device Information:</strong> Browser type, operating system, and device identifiers.</li>
              <li><strong>Usage Data:</strong> Pages visited, time spent on pages, and interaction patterns.</li>
              <li><strong>Log Data:</strong> IP address, access times, and referring URLs.</li>
              <li><strong>Cookies:</strong> Session cookies for authentication and preference cookies for your settings. See Section 7 for details.</li>
            </ul>
          </section>

          {/* How I Use Your Information */}
          <section>
            <h2 className="text-xl font-semibold mb-3">3. How I Use Your Information</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              I use the information I collect for the following purposes:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Provide Services:</strong> To create and manage your account, display your saved shows, and enable show submissions.</li>
              <li><strong>Authentication:</strong> To verify your identity and maintain secure access to your account.</li>
              <li><strong>Communications:</strong> To send you transactional emails (password resets, magic links, email verification) and, with your consent, promotional updates about new features.</li>
              <li><strong>Personalization:</strong> To remember your preferences such as theme, timezone, and language.</li>
              <li><strong>Improvement:</strong> To analyze usage patterns and improve the services.</li>
              <li><strong>Security:</strong> To detect and prevent fraud, abuse, and security incidents.</li>
              <li><strong>Legal Compliance:</strong> To comply with applicable laws and regulations.</li>
            </ul>
          </section>

          {/* Third-Party Services */}
          <section>
            <h2 className="text-xl font-semibold mb-3">4. Third-Party Services and Data Sharing</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              I share your information with the following categories of third parties:
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">4.1 Service Providers</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Resend:</strong> Email delivery service. I share your email address to send transactional and notification emails. <Link href="https://resend.com/legal/privacy-policy" className="underline hover:text-muted-foreground">Resend Privacy Policy</Link></li>
              <li><strong>Railway:</strong> Cloud hosting provider where the application and database are hosted. <Link href="https://railway.app/legal/privacy" className="underline hover:text-muted-foreground">Railway Privacy Policy</Link></li>
              <li><strong>Google Cloud Storage:</strong> Used for backup storage. <Link href="https://cloud.google.com/terms/cloud-privacy-notice" className="underline hover:text-muted-foreground">Google Cloud Privacy Notice</Link></li>
              <li><strong>Anthropic (Claude AI):</strong> I use AI to help extract show details from uploaded flyer images and to discover music links for artists. Uploaded images and artist names may be processed by Anthropic&apos;s Claude API. No personal user information is sent to Anthropic. <Link href="https://www.anthropic.com/privacy" className="underline hover:text-muted-foreground">Anthropic Privacy Policy</Link></li>
              <li><strong>PostHog:</strong> I use PostHog for privacy-friendly product analytics, including page views and session recordings (with all inputs masked). PostHog is only activated after you consent to analytics cookies via the cookie banner. If you decline or revoke consent, no data is sent to PostHog. <Link href="https://posthog.com/privacy" className="underline hover:text-muted-foreground">PostHog Privacy Policy</Link></li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">4.2 Authentication Providers</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Google OAuth:</strong> If you choose to sign in with Google. <Link href="https://policies.google.com/privacy" className="underline hover:text-muted-foreground">Google Privacy Policy</Link></li>
              <li><strong>GitHub OAuth:</strong> If you choose to sign in with GitHub. <Link href="https://docs.github.com/en/site-policy/privacy-policies/github-privacy-statement" className="underline hover:text-muted-foreground">GitHub Privacy Statement</Link></li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">4.3 Embedded Content</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              The site may embed content from third-party music platforms:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Spotify:</strong> Embedded players may set cookies and collect data per <Link href="https://www.spotify.com/legal/privacy-policy/" className="underline hover:text-muted-foreground">Spotify&apos;s Privacy Policy</Link></li>
              <li><strong>Bandcamp:</strong> Embedded players may set cookies and collect data per <Link href="https://bandcamp.com/privacy" className="underline hover:text-muted-foreground">Bandcamp&apos;s Privacy Policy</Link></li>
              <li><strong>SoundCloud:</strong> Embedded players may set cookies and collect data per <Link href="https://soundcloud.com/pages/privacy" className="underline hover:text-muted-foreground">SoundCloud&apos;s Privacy Policy</Link></li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">4.4 Discord Notifications</h3>
            <p className="text-foreground/90 leading-relaxed">
              I use Discord webhooks to send notifications about new show submissions. No personal user data is shared with Discord users beyond what you include in public submissions.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">4.5 I Do NOT Sell Your Data</h3>
            <p className="text-foreground/90 leading-relaxed">
              I do not sell, rent, or trade your personal information to third parties for their marketing purposes. I do not share your data with data brokers.
            </p>
          </section>

          {/* Data Retention */}
          <section>
            <h2 className="text-xl font-semibold mb-3">5. Data Retention</h2>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Active Accounts:</strong> I retain your information for as long as your account is active.</li>
              <li><strong>Deleted Accounts:</strong> When you delete your account, I retain your data for 30 days to allow account recovery if requested. After this grace period, your data is permanently deleted.</li>
              <li><strong>Legal Requirements:</strong> I may retain certain information longer if required by law or to protect legal interests.</li>
              <li><strong>Anonymized Data:</strong> I may retain anonymized, aggregated data indefinitely for analytics purposes.</li>
            </ul>
          </section>

          {/* Your Privacy Rights */}
          <section>
            <h2 className="text-xl font-semibold mb-3">6. Your Privacy Rights</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              Depending on your location, you may have the following rights:
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">6.1 All Users</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Access:</strong> Request a copy of the personal information I hold about you.</li>
              <li><strong>Correction:</strong> Update or correct inaccurate information via your profile settings.</li>
              <li><strong>Deletion:</strong> Delete your account and associated data through your account settings.</li>
              <li><strong>Portability:</strong> Request an export of your data in a machine-readable format.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">6.2 California Residents (CCPA/CPRA)</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              Under the California Consumer Privacy Act, as amended by the California Privacy Rights Act and updated regulations effective January 1, 2026, you have the right to:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Know:</strong> Request disclosure of the categories and specific pieces of personal information I collect.</li>
              <li><strong>Delete:</strong> Request deletion of your personal information.</li>
              <li><strong>Opt-Out of Sale/Sharing:</strong> I do not sell or share your personal information for cross-context behavioral advertising.</li>
              <li><strong>Non-Discrimination:</strong> I will not discriminate against you for exercising your privacy rights.</li>
              <li><strong>Correct:</strong> Request correction of inaccurate personal information.</li>
              <li><strong>Limit Use of Sensitive Personal Information:</strong> I only use sensitive personal information (such as your email and account credentials) for purposes necessary to provide the services.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">6.3 Global Privacy Control (GPC)</h3>
            <p className="text-foreground/90 leading-relaxed">
              I honor Global Privacy Control (GPC) signals. If your browser sends a GPC signal, I will treat it as a valid opt-out request for the sale or sharing of personal information, as required by California law and other state privacy laws effective in 2026 (including Kentucky, Rhode Island, and Indiana).
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">6.4 European Economic Area (GDPR)</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              If you are in the EEA, you have additional rights under the General Data Protection Regulation:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Legal Basis:</strong> I process your data based on: (a) your consent, (b) performance of the contract with you, (c) legitimate interests, or (d) legal obligations.</li>
              <li><strong>Withdraw Consent:</strong> Where I rely on consent, you may withdraw it at any time.</li>
              <li><strong>Restriction:</strong> Request that I restrict processing of your data.</li>
              <li><strong>Object:</strong> Object to processing based on legitimate interests.</li>
              <li><strong>Lodge Complaint:</strong> File a complaint with your local data protection authority.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">6.5 How to Exercise Your Rights</h3>
            <p className="text-foreground/90 leading-relaxed">
              To exercise any of these rights, please contact me at <Link href="mailto:hello@psychichomily.com" className="underline hover:text-muted-foreground">hello@psychichomily.com</Link>. I will respond to your request within 45 days (or 30 days for GDPR requests). I may need to verify your identity before processing your request.
            </p>
          </section>

          {/* Cookies */}
          <section>
            <h2 className="text-xl font-semibold mb-3">7. Cookies and Tracking Technologies</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">7.1 Cookies I Use</h3>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Essential Cookies:</strong> Required for authentication and security. These cannot be disabled without breaking core functionality.</li>
              <li><strong>Preference Cookies:</strong> Remember your settings like theme preference. These improve your experience but are not strictly necessary.</li>
              <li><strong>Analytics Cookies:</strong> With your consent, I use PostHog to collect anonymized usage data such as page views and session recordings (with all inputs masked). These cookies are only set after you accept analytics via the cookie consent banner. You can change your preference at any time through the cookie preferences link in the footer.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">7.2 Third-Party Cookies</h3>
            <p className="text-foreground/90 leading-relaxed">
              Embedded content from Spotify, Bandcamp, and SoundCloud may set their own cookies. These are governed by their respective privacy policies. You can manage third-party cookies through your browser settings.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">7.3 Managing Cookies</h3>
            <p className="text-foreground/90 leading-relaxed">
              Most browsers allow you to control cookies through their settings. Note that disabling essential cookies may prevent you from using authenticated features of the service.
            </p>
          </section>

          {/* Security */}
          <section>
            <h2 className="text-xl font-semibold mb-3">8. Data Security</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              I implement appropriate technical and organizational measures to protect your personal information:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li><strong>Encryption:</strong> All data transmitted between your browser and the servers is encrypted using TLS/HTTPS.</li>
              <li><strong>Password Security:</strong> Passwords are hashed using bcrypt. I check passwords against known breach databases (HaveIBeenPwned) to prevent use of compromised credentials.</li>
              <li><strong>Authentication:</strong> The service supports secure authentication methods including passkeys/WebAuthn, magic links, and OAuth.</li>
              <li><strong>Access Controls:</strong> Access to personal data is restricted to authorized personnel only.</li>
              <li><strong>Backups:</strong> Regular encrypted backups ensure data recovery in case of incidents.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              While I strive to protect your information, no method of transmission over the Internet is 100% secure. I cannot guarantee absolute security.
            </p>
          </section>

          {/* Children's Privacy */}
          <section>
            <h2 className="text-xl font-semibold mb-3">9. Children&apos;s Privacy</h2>
            <p className="text-foreground/90 leading-relaxed">
              The services are not directed to individuals under the age of 16. I do not knowingly collect personal information from children under 16. If you are a parent or guardian and believe your child has provided personal information, please contact me at <Link href="mailto:hello@psychichomily.com" className="underline hover:text-muted-foreground">hello@psychichomily.com</Link> and I will delete such information.
            </p>
          </section>

          {/* International Transfers */}
          <section>
            <h2 className="text-xl font-semibold mb-3">10. International Data Transfers</h2>
            <p className="text-foreground/90 leading-relaxed">
              The servers are located in the United States. If you access the services from outside the United States, your information will be transferred to, stored, and processed in the United States. By using the services, you consent to this transfer. I take steps to ensure that your data receives adequate protection in accordance with this Privacy Policy.
            </p>
          </section>

          {/* Changes to Policy */}
          <section>
            <h2 className="text-xl font-semibold mb-3">11. Changes to This Privacy Policy</h2>
            <p className="text-foreground/90 leading-relaxed">
              I may update this Privacy Policy from time to time. I will notify you of any material changes by posting the new Privacy Policy on this page and updating the &quot;Last Updated&quot; date. For significant changes, I may also send you an email notification. Your continued use of the services after any changes indicates your acceptance of the updated policy.
            </p>
          </section>

          {/* Contact */}
          <section>
            <h2 className="text-xl font-semibold mb-3">12. Contact Us</h2>
            <p className="text-foreground/90 leading-relaxed">
              If you have any questions about this Privacy Policy or privacy practices, please contact me at <Link href="mailto:hello@psychichomily.com" className="underline hover:text-muted-foreground">hello@psychichomily.com</Link>.
            </p>
          </section>

          {/* Summary Table */}
          <section className="mt-8 pt-6 border-t border-border">
            <h2 className="text-xl font-semibold mb-3">Quick Reference: Your Rights by Location</h2>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border">
                    <th className="text-left py-2 pr-4">Right</th>
                    <th className="text-center py-2 px-2">All Users</th>
                    <th className="text-center py-2 px-2">California</th>
                    <th className="text-center py-2 px-2">EEA</th>
                  </tr>
                </thead>
                <tbody className="text-foreground/90">
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Access your data</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Correct your data</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Delete your data</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Export your data</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Opt-out of sale/sharing</td>
                    <td className="text-center py-2 px-2">N/A*</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr className="border-b border-border/50">
                    <td className="py-2 pr-4">Restrict processing</td>
                    <td className="text-center py-2 px-2">—</td>
                    <td className="text-center py-2 px-2">—</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                  <tr>
                    <td className="py-2 pr-4">Lodge complaint with authority</td>
                    <td className="text-center py-2 px-2">—</td>
                    <td className="text-center py-2 px-2">✓</td>
                    <td className="text-center py-2 px-2">✓</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p className="text-sm text-muted-foreground mt-2">
              * I do not sell or share personal information, so opt-out is not applicable.
            </p>
          </section>
        </div>

        <div className="mt-12 pt-6 border-t border-border text-center text-sm text-muted-foreground">
          <Link href="/terms" className="underline hover:text-foreground">
            Terms of Service
          </Link>
          {' · '}
          <Link href="/" className="underline hover:text-foreground">
            Return to Home
          </Link>
        </div>
      </main>
    </div>
  )
}
