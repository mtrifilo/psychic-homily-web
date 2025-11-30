## Show Submission feature

- manual form

When adding a new artist, as the user types, we want to seperately search the database for matching artists to recommend using shadcn's Command component.

```bash
pnpm dlx shadcn@latest add command
```

Same idea when adding venues.

This will allow users to simply click on matching artists so that they don't have to type the whole thing, and in the code, this will allow us to know the artist ID of existing artists to use, and to avoid duplicates. If an artist is not found in the database (for example, a new artist not yet in the data), then the user will type the whole name, and no ID will be passed to the backend. The backend can then handle creating a new artist in the database with an ID. Once added, users adding the same artist to anothe show will then see that artist show up in the search results as they type.

We need to implement the following.

1. A Postgres database instance in Docker - DONE
2. A connection to the new postgres database using Gorm in the Go backend application (docker-compose file exists)
3. Set up golang-migrate to start creating the initial tables (artist, venue, user, etc.) properly using the migration code
4. Ingress the data from the Hugo Yaml files for Artists and Venues
5. Implement search functionality as a user types, so that they can select an existing artist as they add them to a new show submission.

- Importantly, we need to figure out how we can search the backend data efficiently for a good UX. Users need to type in real-time without lag, and see matching artists as they type as close to real-time as possible. For example, if they type "The Fl", and there is a band in the database called "The Flying Cows", then "They Flying Cows" should appear in the command component below the input for the user to select. Upon clicking the match, the input should show the complete name, and the artist data on the form state should include the ID of "The Flying Cows" until the user changes the field's value to something else.

6. Set up logic to re-generate the Hugo yaml data files using the postgres "sourece of truth" data. We don't need to show artists on the front end for shows if certain artists have no shows coming up. This will allow us to add as many artists to the database as we want, without bogging down the front end by adding all of them (active/inactive artists) in the Hugo data files for rendering
