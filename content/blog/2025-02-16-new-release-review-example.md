---
title: "PsychicHomily.com is now live!"
date: 2025-02-16
categories: ["Psychic Homily"]
description: "A detailed review of the latest release..."
---

Welcome to the shiny new Psychic Homily website ✨

This project has been in the works for the past few weeks as a labor of love to build a show list and blog to help bolster the AZ music community, and hopefully get more people to come out to shows. It's a real challenge to keep up with all of the show announcements and new releases on Instagram alone, and for those who don't look at Instagram every day, a lot of those show announcements will be missed. Why is Instagram so important? It seems to be the most reliable and active social media platform for keeping up with artists, for better or worse. In the past, alt-weekly's did a great job of listing excellent shows, but in modern times, there's so much noise and non-music events to sift through to find the truly special and unmissable show announcements. That's a void this website will hopefully help fill, along with others I may not have encountered.

In addition to the show list, there will also be a blog featuring new releases worth hearing in a similar short-form format to the Psychic Homily Substack. The Substack isn't going anywhere, but the blog will allow for more frequent and shorter posts which could be compiled into a larger newsletter going forward. Taste is subjective, so these features may not be for you, and that's fine! The goal has always been "to document and amplify some of the most exciting and memorable new music releases, shows, and cultural events from Arizona musicians and beyond, focusing on artists truly putting their hearts and souls into their work bravely." That is the mission statement, manifesto, promise, and ultimate goal.

A little about the technical side of this Web Site for the nerds. PsychicHomily.com is built with Hugo (a Go-based static site generator) extending the [Ananke](https://github.com/theNewDynamic/gohugo-theme-ananke) theme, along with some helper scripts in JavaScript running with NodeJS. The show listing markup is generated using a CLI agentic AI workflow running Claude's Sonnet 3.5 model (in private mode) to format the show listings based on varying and unpredictable text formats as inputs, and to generate the markup files for each listing. This takes a lot of the manual tedium out of working with markup-based data in this [Jamstack](https://jamstack.org/) project. All of the content is statically generated at compile time for the best performance and caching possible. So far, JavaScript is only used for the helper scripts for the dev (me). The codebase is open source and available on [GitHub](https://github.com/mtrifilo/psychic-homily-web). Feel free to open an issue if you find any bugs!
