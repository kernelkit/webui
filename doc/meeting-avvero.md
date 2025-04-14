Agenda TRS
==========

Second meeting to go through the technical requirements.

- Timeline
  - Collaborative Project Planning on GitHub (proposal)

- eBGP/VXLAN seems to require VRF support
  - We recommend a timeboxed prestudy

- Integrity/Authenticity of firmware and .conf files on USB?
  - Maybe separate workshop or prestudy?

- Responsibility: who does what?
  - Avvero/Minex previously wanted to take lead on ModemManager
  - Avvero/Minex need to handle testing
  - Addiva can help with reviewing and discussing YANG model and,
    C code for YANG -> modem-manager.conf and other integration
  - Addiva could also build test lab for LTE testing, separate discussion


Tasks
-----

 1. Avvero take lead on ModemManager
 2. Addiva time-boxed pre-study eVPN
 3. Addiva come back with basic firewall model
    - masquerading
    - port forwarding
 4. Proposed 
 5. How to work with "templates" for end-users?
    - Windows tooling/Device Managers
	- Stand-alone CLI for working on files



Prestudy (3 MW)
---------------

Calendar time  : 1.5 weeks
Man hours (max): 3   weeks

 - One week: BGP + eVPN investigate integration in Infix (one person)
 - Few days: encryption and unattended upgrades (two ppl)
 - Few days: prototype firewall model (two ppl)

Outcome: implementation proposal/task in GitHub Project

Findings to be presented at a demo over Teams.

### Goal

Figure out scope, risks, and arrive at an estimate of total work
incl. regression tests, design/code reviews, support.

### Ideas

 - Use ietf-keystore symmetric keys for encryption. Same key can be
   deploys in all devices in an installation.
 - Use zip files with password?
 - USe Bitlocker for entire USB key?


