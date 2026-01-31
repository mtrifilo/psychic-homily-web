import { Metadata } from 'next'
import Link from 'next/link'

export const metadata: Metadata = {
  title: 'Terms of Service | Psychic Homily',
  description: 'Terms of Service for Psychic Homily - the rules and guidelines for using our platform.',
}

export default function TermsOfServicePage() {
  const lastUpdated = 'January 29, 2026'
  const effectiveDate = 'January 29, 2026'

  return (
    <div className="flex min-h-screen items-start justify-center bg-background">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Terms of Service</h1>
        <p className="text-center text-muted-foreground mb-8">
          Last Updated: {lastUpdated} | Effective: {effectiveDate}
        </p>

        {/* Important Notice Box */}
        <div className="bg-muted/50 border border-border rounded-lg p-4 mb-8">
          <p className="text-sm font-medium mb-2">IMPORTANT: PLEASE READ THESE TERMS CAREFULLY</p>
          <p className="text-sm text-foreground/90">
            These Terms contain an <strong>arbitration agreement</strong> (Section 14) that affects your legal rights.
            By using Psychic Homily, you agree to resolve disputes through binding arbitration rather than in court,
            unless you opt out within 30 days of creating your account.
          </p>
        </div>

        <div className="prose prose-neutral dark:prose-invert max-w-none space-y-8">
          {/* 1. Acceptance of Terms */}
          <section>
            <h2 className="text-xl font-semibold mb-3">1. Acceptance of Terms</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              Welcome to Psychic Homily. These Terms of Service (&quot;Terms&quot;) constitute a legally binding agreement between you (&quot;you&quot; or &quot;User&quot;) and Psychic Homily (&quot;we,&quot; &quot;us,&quot; or &quot;our&quot;) governing your access to and use of the Psychic Homily website, applications, and services (collectively, the &quot;Service&quot;).
            </p>
            <p className="text-foreground/90 leading-relaxed mb-3">
              By creating an account, accessing, or using the Service, you acknowledge that you have read, understood, and agree to be bound by these Terms and our <Link href="/privacy" className="underline hover:text-muted-foreground">Privacy Policy</Link>, which is incorporated herein by reference.
            </p>
            <p className="text-foreground/90 leading-relaxed">
              If you do not agree to these Terms, you may not access or use the Service. If you are using the Service on behalf of an organization, you represent and warrant that you have the authority to bind that organization to these Terms.
            </p>
          </section>

          {/* 2. Eligibility */}
          <section>
            <h2 className="text-xl font-semibold mb-3">2. Eligibility</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              You must be at least 16 years of age to use the Service. By using the Service, you represent and warrant that:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>You are at least 16 years old;</li>
              <li>You have the legal capacity to enter into these Terms;</li>
              <li>You are not prohibited from using the Service under any applicable law;</li>
              <li>Your use of the Service will not violate any applicable law or regulation.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              If you are between 16 and 18 years of age (or the age of majority in your jurisdiction), you may only use the Service with the consent of a parent or legal guardian who agrees to be bound by these Terms.
            </p>
          </section>

          {/* 3. Description of Service */}
          <section>
            <h2 className="text-xl font-semibold mb-3">3. Description of Service</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              Psychic Homily is a platform for discovering live music shows and events. The Service allows users to:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>Browse and search for upcoming music shows and events;</li>
              <li>Save shows to a personal collection;</li>
              <li>Submit new shows and events for listing;</li>
              <li>Access information about artists and venues;</li>
              <li>Read blog content about music news and reviews.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              We reserve the right to modify, suspend, or discontinue any aspect of the Service at any time without notice or liability.
            </p>
          </section>

          {/* 4. User Accounts */}
          <section>
            <h2 className="text-xl font-semibold mb-3">4. User Accounts</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">4.1 Account Creation</h3>
            <p className="text-foreground/90 leading-relaxed">
              To access certain features of the Service, you must create an account. You may register using an email address and password, a magic link, a passkey, or through third-party authentication providers (Google, GitHub). You agree to provide accurate, current, and complete information during registration.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">4.2 Account Security</h3>
            <p className="text-foreground/90 leading-relaxed">
              You are responsible for maintaining the confidentiality of your account credentials and for all activities that occur under your account. You agree to: (a) immediately notify us of any unauthorized use of your account; (b) ensure you log out of your account at the end of each session; and (c) not share your account credentials with any third party.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">4.3 Account Termination</h3>
            <p className="text-foreground/90 leading-relaxed">
              You may delete your account at any time through your account settings. Upon deletion, your account will enter a 30-day grace period during which you may request account recovery. After this period, your account and associated data will be permanently deleted in accordance with our <Link href="/privacy" className="underline hover:text-muted-foreground">Privacy Policy</Link>.
            </p>
          </section>

          {/* 5. User-Generated Content */}
          <section>
            <h2 className="text-xl font-semibold mb-3">5. User-Generated Content</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">5.1 Your Content</h3>
            <p className="text-foreground/90 leading-relaxed">
              &quot;User Content&quot; means any content you submit, post, or transmit through the Service, including but not limited to show submissions, profile information, and any other materials. You retain ownership of your User Content, subject to the license granted below.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">5.2 License to Us</h3>
            <p className="text-foreground/90 leading-relaxed">
              By submitting User Content, you grant us a worldwide, non-exclusive, royalty-free, sublicensable, and transferable license to use, reproduce, modify, adapt, publish, translate, distribute, and display such User Content in connection with operating and providing the Service. This license continues even if you stop using the Service, but only for User Content that has been made public or shared with others.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">5.3 Your Representations</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              You represent and warrant that:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>You own or have the necessary rights to submit the User Content;</li>
              <li>Your User Content does not infringe any third party&apos;s intellectual property or other rights;</li>
              <li>Your User Content complies with these Terms and all applicable laws;</li>
              <li>Your User Content is accurate and not misleading.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">5.4 Content Removal</h3>
            <p className="text-foreground/90 leading-relaxed">
              We reserve the right, but have no obligation, to monitor, review, or remove User Content at our sole discretion, without notice, for any reason, including but not limited to violations of these Terms or applicable law.
            </p>
          </section>

          {/* 6. Acceptable Use Policy */}
          <section>
            <h2 className="text-xl font-semibold mb-3">6. Acceptable Use Policy</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              You agree not to use the Service to:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>Violate any applicable law, regulation, or these Terms;</li>
              <li>Infringe the intellectual property rights of others;</li>
              <li>Submit false, misleading, or inaccurate information about shows, artists, or venues;</li>
              <li>Harass, abuse, threaten, or intimidate other users;</li>
              <li>Post content that is defamatory, obscene, pornographic, or promotes violence or discrimination;</li>
              <li>Spam, phish, or distribute malware;</li>
              <li>Attempt to gain unauthorized access to the Service or other users&apos; accounts;</li>
              <li>Interfere with or disrupt the Service or servers;</li>
              <li>Use automated means (bots, scrapers) to access the Service without our written permission;</li>
              <li>Collect or harvest user information without consent;</li>
              <li>Impersonate any person or entity;</li>
              <li>Use the Service for any commercial purpose without our prior written consent.</li>
            </ul>
          </section>

          {/* 7. Intellectual Property */}
          <section>
            <h2 className="text-xl font-semibold mb-3">7. Intellectual Property</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">7.1 Our Intellectual Property</h3>
            <p className="text-foreground/90 leading-relaxed">
              The Service and its original content (excluding User Content), features, and functionality are owned by Psychic Homily and are protected by copyright, trademark, and other intellectual property laws. Our trademarks and trade dress may not be used without our prior written permission.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">7.2 Limited License</h3>
            <p className="text-foreground/90 leading-relaxed">
              Subject to your compliance with these Terms, we grant you a limited, non-exclusive, non-transferable, revocable license to access and use the Service for your personal, non-commercial purposes.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">7.3 Feedback</h3>
            <p className="text-foreground/90 leading-relaxed">
              If you provide us with feedback, suggestions, or ideas about the Service, you grant us the right to use such feedback without restriction or compensation to you.
            </p>
          </section>

          {/* 8. Copyright and DMCA */}
          <section>
            <h2 className="text-xl font-semibold mb-3">8. Copyright Policy (DMCA)</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              We respect the intellectual property rights of others and expect our users to do the same. In accordance with the Digital Millennium Copyright Act (&quot;DMCA&quot;), we will respond to notices of alleged copyright infringement that comply with the DMCA.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">8.1 Filing a DMCA Notice</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              If you believe that content on the Service infringes your copyright, please send a notice to our designated agent containing:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>A physical or electronic signature of the copyright owner or authorized agent;</li>
              <li>Identification of the copyrighted work claimed to be infringed;</li>
              <li>Identification of the material claimed to be infringing, with sufficient detail for us to locate it;</li>
              <li>Your contact information (address, telephone number, email);</li>
              <li>A statement that you have a good faith belief that the use is not authorized;</li>
              <li>A statement, under penalty of perjury, that the information is accurate and you are authorized to act on behalf of the copyright owner.</li>
            </ul>

            <h3 className="text-lg font-medium mt-4 mb-2">8.2 DMCA Agent</h3>
            <p className="text-foreground/90 leading-relaxed">
              Send DMCA notices to: <Link href="mailto:dmca@psychichomily.com" className="underline hover:text-muted-foreground">dmca@psychichomily.com</Link>
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">8.3 Counter-Notification</h3>
            <p className="text-foreground/90 leading-relaxed">
              If you believe your content was wrongly removed, you may submit a counter-notification containing: (a) your physical or electronic signature; (b) identification of the removed material and its prior location; (c) a statement under penalty of perjury that you have a good faith belief the material was removed by mistake; (d) your contact information; and (e) a statement consenting to jurisdiction in federal court.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">8.4 Repeat Infringers</h3>
            <p className="text-foreground/90 leading-relaxed">
              We will terminate the accounts of users who are repeat copyright infringers.
            </p>
          </section>

          {/* 9. Third-Party Services */}
          <section>
            <h2 className="text-xl font-semibold mb-3">9. Third-Party Services and Links</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              The Service may contain links to third-party websites, services, or content, including but not limited to:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>Embedded music players from Spotify, Bandcamp, and SoundCloud;</li>
              <li>Authentication services from Google and GitHub;</li>
              <li>Links to venue websites and ticket purchasing platforms.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              We do not control and are not responsible for the content, privacy policies, or practices of third-party services. Your interactions with third-party services are governed by their respective terms and policies.
            </p>
          </section>

          {/* 10. Artist and Venue Information */}
          <section>
            <h2 className="text-xl font-semibold mb-3">10. Artist and Venue Information</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">10.1 Nature of Information</h3>
            <p className="text-foreground/90 leading-relaxed">
              Psychic Homily publishes factual information about live music events, including artist names, venue names, event dates, times, and related details. This information is published for informational purposes to help music fans discover local shows. We do not claim any ownership of artist or venue names, trademarks, or intellectual property.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">10.2 Information Sources</h3>
            <p className="text-foreground/90 leading-relaxed">
              Event information on our platform comes from: (a) user submissions; (b) publicly available sources such as venue websites, social media, and press releases; and (c) direct submissions from artists, venues, or promoters. We strive for accuracy but cannot guarantee that all information is complete or current.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">10.3 Content Correction Requests</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              If you are an artist, venue, or promoter and you believe information about your events is inaccurate, outdated, or incomplete, you may request a correction by contacting us at <Link href="mailto:corrections@psychichomily.com" className="underline hover:text-muted-foreground">corrections@psychichomily.com</Link>. Please include:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>Your name and role (artist, venue representative, promoter, etc.);</li>
              <li>The specific listing(s) requiring correction;</li>
              <li>The correct information or details of the error;</li>
              <li>Contact information so we can verify your identity if needed.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              We will review correction requests promptly and make reasonable efforts to update or correct information within 7 business days.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">10.4 Artist and Venue Removal Requests</h3>
            <p className="text-foreground/90 leading-relaxed mb-2">
              We believe that listing publicly announced shows serves the public interest by helping fans discover live music. However, we respect that artists and venues may have legitimate reasons to request removal of their information. You may request removal if:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>The event has been cancelled;</li>
              <li>The event is private and was listed in error;</li>
              <li>You have safety or privacy concerns;</li>
              <li>The listing contains materially false information that cannot be corrected;</li>
              <li>Other legitimate reasons at our discretion.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              To request removal, email <Link href="mailto:corrections@psychichomily.com" className="underline hover:text-muted-foreground">corrections@psychichomily.com</Link> with the subject line &quot;Removal Request&quot; and include verification of your identity and relationship to the event. We will review requests on a case-by-case basis and respond within 14 business days.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">10.5 No Implied Endorsement</h3>
            <p className="text-foreground/90 leading-relaxed">
              The inclusion of any artist, venue, or event on our platform does not imply endorsement by that artist or venue of Psychic Homily, nor does it imply our endorsement of any artist, venue, or event. We are an independent informational service.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">10.6 Artist and Venue Images</h3>
            <p className="text-foreground/90 leading-relaxed">
              We may display artist photos, venue images, or promotional materials in connection with event listings. If you are the copyright owner of an image and wish to have it removed, please submit a DMCA notice as described in Section 8. If you are an artist and object to the use of your likeness for non-copyright reasons, please contact us at <Link href="mailto:corrections@psychichomily.com" className="underline hover:text-muted-foreground">corrections@psychichomily.com</Link> with the subject line &quot;Image Removal Request.&quot;
            </p>
          </section>

          {/* 11. Disclaimers */}
          <section>
            <h2 className="text-xl font-semibold mb-3">11. Disclaimers</h2>
            <p className="text-foreground/90 leading-relaxed mb-3 uppercase text-sm">
              THE SERVICE IS PROVIDED &quot;AS IS&quot; AND &quot;AS AVAILABLE&quot; WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO IMPLIED WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, TITLE, AND NON-INFRINGEMENT.
            </p>
            <p className="text-foreground/90 leading-relaxed mb-3">
              We do not warrant that:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>The Service will be uninterrupted, secure, or error-free;</li>
              <li>The information provided through the Service (including show times, venues, and artist information) is accurate, complete, or current;</li>
              <li>Shows or events listed on the Service will actually occur as described;</li>
              <li>Any errors or defects will be corrected.</li>
            </ul>
            <p className="text-foreground/90 leading-relaxed mt-3">
              You acknowledge that show information is often submitted by users or gathered from third-party sources, and we cannot guarantee its accuracy. Always verify event details with the venue or official sources before attending.
            </p>
          </section>

          {/* 12. Limitation of Liability */}
          <section>
            <h2 className="text-xl font-semibold mb-3">12. Limitation of Liability</h2>
            <p className="text-foreground/90 leading-relaxed mb-3 uppercase text-sm">
              TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, IN NO EVENT SHALL PSYCHIC HOMILY, ITS OFFICERS, DIRECTORS, EMPLOYEES, AGENTS, OR AFFILIATES BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL, OR PUNITIVE DAMAGES, INCLUDING BUT NOT LIMITED TO LOSS OF PROFITS, DATA, USE, OR GOODWILL, ARISING OUT OF OR RELATED TO YOUR USE OF THE SERVICE.
            </p>
            <p className="text-foreground/90 leading-relaxed mb-3 uppercase text-sm">
              OUR TOTAL LIABILITY FOR ANY CLAIMS ARISING FROM OR RELATED TO THESE TERMS OR THE SERVICE SHALL NOT EXCEED THE GREATER OF: (A) THE AMOUNT YOU PAID US, IF ANY, IN THE TWELVE (12) MONTHS PRIOR TO THE CLAIM; OR (B) ONE HUNDRED DOLLARS ($100).
            </p>
            <p className="text-foreground/90 leading-relaxed">
              Some jurisdictions do not allow the exclusion or limitation of certain damages, so some of the above limitations may not apply to you. In such cases, our liability will be limited to the fullest extent permitted by applicable law.
            </p>
          </section>

          {/* 13. Indemnification */}
          <section>
            <h2 className="text-xl font-semibold mb-3">13. Indemnification</h2>
            <p className="text-foreground/90 leading-relaxed">
              You agree to indemnify, defend, and hold harmless Psychic Homily and its officers, directors, employees, agents, and affiliates from and against any claims, liabilities, damages, losses, costs, or expenses (including reasonable attorneys&apos; fees) arising out of or related to: (a) your use of the Service; (b) your User Content; (c) your violation of these Terms; or (d) your violation of any rights of another party.
            </p>
          </section>

          {/* 14. Dispute Resolution */}
          <section>
            <h2 className="text-xl font-semibold mb-3">14. Dispute Resolution and Arbitration</h2>
            <p className="text-foreground/90 leading-relaxed mb-3 font-medium">
              PLEASE READ THIS SECTION CAREFULLY. IT AFFECTS YOUR LEGAL RIGHTS, INCLUDING YOUR RIGHT TO FILE A LAWSUIT IN COURT.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">14.1 Informal Resolution</h3>
            <p className="text-foreground/90 leading-relaxed">
              Before initiating any arbitration or court proceeding, you agree to first contact us at <Link href="mailto:legal@psychichomily.com" className="underline hover:text-muted-foreground">legal@psychichomily.com</Link> and attempt to resolve the dispute informally for at least 30 days.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">14.2 Binding Arbitration</h3>
            <p className="text-foreground/90 leading-relaxed">
              If informal resolution fails, any dispute, claim, or controversy arising out of or relating to these Terms or the Service shall be resolved by binding arbitration administered by the American Arbitration Association (&quot;AAA&quot;) under its Consumer Arbitration Rules. The arbitration will be conducted in English, and the arbitrator&apos;s decision will be final and binding.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">14.3 Class Action Waiver</h3>
            <p className="text-foreground/90 leading-relaxed">
              YOU AND PSYCHIC HOMILY AGREE THAT DISPUTES WILL ONLY BE ARBITRATED ON AN INDIVIDUAL BASIS AND NOT AS A CLASS ACTION, COLLECTIVE ACTION, OR REPRESENTATIVE ACTION. The arbitrator may not consolidate claims or preside over any form of class or representative proceeding.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">14.4 Exceptions</h3>
            <p className="text-foreground/90 leading-relaxed">
              Notwithstanding the above, either party may: (a) bring an individual action in small claims court; (b) seek injunctive relief in any court of competent jurisdiction for actual or threatened infringement of intellectual property rights.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">14.5 Opt-Out Right</h3>
            <p className="text-foreground/90 leading-relaxed">
              You may opt out of this arbitration agreement by sending written notice to <Link href="mailto:legal@psychichomily.com" className="underline hover:text-muted-foreground">legal@psychichomily.com</Link> within 30 days of creating your account. Your notice must include your name, email address, and a clear statement that you wish to opt out of the arbitration agreement. If you opt out, you may pursue claims in court.
            </p>
          </section>

          {/* 15. Termination */}
          <section>
            <h2 className="text-xl font-semibold mb-3">15. Termination</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              We may suspend or terminate your access to the Service at any time, with or without cause, with or without notice, including if we reasonably believe you have violated these Terms. Upon termination:
            </p>
            <ul className="list-disc pl-6 space-y-2 text-foreground/90">
              <li>Your right to use the Service will immediately cease;</li>
              <li>We may delete your account and User Content;</li>
              <li>Provisions that by their nature should survive termination will survive, including Sections 5.2, 7, 8, 11, 12, 13, 14, and 16.</li>
            </ul>
          </section>

          {/* 16. Governing Law */}
          <section>
            <h2 className="text-xl font-semibold mb-3">16. Governing Law</h2>
            <p className="text-foreground/90 leading-relaxed">
              These Terms shall be governed by and construed in accordance with the laws of the State of Arizona, United States, without regard to its conflict of law principles. For any disputes not subject to arbitration, you consent to the exclusive jurisdiction of the state and federal courts located in Maricopa County, Arizona.
            </p>
          </section>

          {/* 17. Changes to Terms */}
          <section>
            <h2 className="text-xl font-semibold mb-3">17. Changes to These Terms</h2>
            <p className="text-foreground/90 leading-relaxed">
              We reserve the right to modify these Terms at any time. If we make material changes, we will notify you by posting the updated Terms on this page and updating the &quot;Last Updated&quot; date. For significant changes, we may also provide additional notice (such as an email notification). Your continued use of the Service after the effective date of the revised Terms constitutes your acceptance of the changes.
            </p>
          </section>

          {/* 18. General Provisions */}
          <section>
            <h2 className="text-xl font-semibold mb-3">18. General Provisions</h2>

            <h3 className="text-lg font-medium mt-4 mb-2">18.1 Entire Agreement</h3>
            <p className="text-foreground/90 leading-relaxed">
              These Terms, together with our Privacy Policy, constitute the entire agreement between you and Psychic Homily regarding the Service and supersede all prior agreements.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">18.2 Severability</h3>
            <p className="text-foreground/90 leading-relaxed">
              If any provision of these Terms is found to be unenforceable, the remaining provisions will continue in full force and effect.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">18.3 Waiver</h3>
            <p className="text-foreground/90 leading-relaxed">
              Our failure to enforce any right or provision of these Terms will not be considered a waiver of that right or provision.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">18.4 Assignment</h3>
            <p className="text-foreground/90 leading-relaxed">
              You may not assign or transfer these Terms without our prior written consent. We may assign our rights and obligations under these Terms without restriction.
            </p>

            <h3 className="text-lg font-medium mt-4 mb-2">18.5 Force Majeure</h3>
            <p className="text-foreground/90 leading-relaxed">
              We will not be liable for any failure or delay in performance due to circumstances beyond our reasonable control, including natural disasters, war, terrorism, labor disputes, or internet service failures.
            </p>
          </section>

          {/* 19. Contact */}
          <section>
            <h2 className="text-xl font-semibold mb-3">19. Contact Us</h2>
            <p className="text-foreground/90 leading-relaxed mb-3">
              If you have any questions about these Terms, please contact us:
            </p>
            <ul className="list-none space-y-2 text-foreground/90">
              <li><strong>General inquiries:</strong> <Link href="mailto:hello@psychichomily.com" className="underline hover:text-muted-foreground">hello@psychichomily.com</Link></li>
              <li><strong>Legal matters:</strong> <Link href="mailto:legal@psychichomily.com" className="underline hover:text-muted-foreground">legal@psychichomily.com</Link></li>
              <li><strong>DMCA notices:</strong> <Link href="mailto:dmca@psychichomily.com" className="underline hover:text-muted-foreground">dmca@psychichomily.com</Link></li>
              <li><strong>Privacy requests:</strong> <Link href="mailto:privacy@psychichomily.com" className="underline hover:text-muted-foreground">privacy@psychichomily.com</Link></li>
              <li><strong>Content corrections/removals:</strong> <Link href="mailto:corrections@psychichomily.com" className="underline hover:text-muted-foreground">corrections@psychichomily.com</Link></li>
            </ul>
          </section>
        </div>

        <div className="mt-12 pt-6 border-t border-border text-center text-sm text-muted-foreground">
          <Link href="/privacy" className="underline hover:text-foreground">
            Privacy Policy
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
