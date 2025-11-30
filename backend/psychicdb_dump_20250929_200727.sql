--
-- PostgreSQL database dump
--

\restrict afosmmn5QaWc9wj5vcZvIBBuTPaKzpPrSUtLGskEnW08dEjGvVysDictcQYJgJu

-- Dumped from database version 17.6 (Debian 17.6-1.pgdg13+1)
-- Dumped by pg_dump version 17.6 (Debian 17.6-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: artists; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.artists (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    state character varying(10),
    city character varying(255),
    instagram character varying(255),
    facebook character varying(255),
    twitter character varying(255),
    youtube character varying(255),
    spotify character varying(255),
    soundcloud character varying(255),
    bandcamp character varying(255),
    website character varying(255),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.artists OWNER TO psychicadmin;

--
-- Name: TABLE artists; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.artists IS 'Musical artists and bands';


--
-- Name: artists_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.artists_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.artists_id_seq OWNER TO psychicadmin;

--
-- Name: artists_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.artists_id_seq OWNED BY public.artists.id;


--
-- Name: oauth_accounts; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.oauth_accounts (
    id integer NOT NULL,
    user_id integer NOT NULL,
    provider character varying(50) NOT NULL,
    provider_user_id character varying(255) NOT NULL,
    provider_email character varying(255),
    provider_name character varying(255),
    provider_avatar_url character varying(500),
    access_token text,
    refresh_token text,
    expires_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.oauth_accounts OWNER TO psychicadmin;

--
-- Name: TABLE oauth_accounts; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.oauth_accounts IS 'OAuth provider connections (Goth compatible)';


--
-- Name: COLUMN oauth_accounts.provider; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON COLUMN public.oauth_accounts.provider IS 'OAuth provider name (google, github, etc.)';


--
-- Name: COLUMN oauth_accounts.provider_user_id; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON COLUMN public.oauth_accounts.provider_user_id IS 'External provider user ID';


--
-- Name: oauth_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.oauth_accounts_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.oauth_accounts_id_seq OWNER TO psychicadmin;

--
-- Name: oauth_accounts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.oauth_accounts_id_seq OWNED BY public.oauth_accounts.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


ALTER TABLE public.schema_migrations OWNER TO psychicadmin;

--
-- Name: show_artists; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.show_artists (
    show_id integer NOT NULL,
    artist_id integer NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    set_type character varying(50) DEFAULT 'performer'::character varying
);


ALTER TABLE public.show_artists OWNER TO psychicadmin;

--
-- Name: TABLE show_artists; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.show_artists IS 'Many-to-many relationship between shows and artists';


--
-- Name: show_venues; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.show_venues (
    show_id integer NOT NULL,
    venue_id integer NOT NULL
);


ALTER TABLE public.show_venues OWNER TO psychicadmin;

--
-- Name: shows; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.shows (
    id integer NOT NULL,
    title character varying(500),
    event_date timestamp without time zone NOT NULL,
    city character varying(255),
    state character varying(10),
    price numeric(10,2),
    age_requirement character varying(255),
    description text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.shows OWNER TO psychicadmin;

--
-- Name: TABLE shows; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.shows IS 'Concert events and performances';


--
-- Name: shows_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.shows_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.shows_id_seq OWNER TO psychicadmin;

--
-- Name: shows_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.shows_id_seq OWNED BY public.shows.id;


--
-- Name: user_preferences; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.user_preferences (
    id integer NOT NULL,
    user_id integer NOT NULL,
    notification_email boolean DEFAULT true NOT NULL,
    notification_push boolean DEFAULT false NOT NULL,
    theme character varying(50) DEFAULT 'light'::character varying NOT NULL,
    timezone character varying(50) DEFAULT 'UTC'::character varying NOT NULL,
    language character varying(10) DEFAULT 'en'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.user_preferences OWNER TO psychicadmin;

--
-- Name: TABLE user_preferences; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.user_preferences IS 'User preferences and settings';


--
-- Name: user_preferences_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.user_preferences_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.user_preferences_id_seq OWNER TO psychicadmin;

--
-- Name: user_preferences_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.user_preferences_id_seq OWNED BY public.user_preferences.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.users (
    id integer NOT NULL,
    email character varying(255),
    username character varying(100),
    password_hash character varying(255),
    first_name character varying(100),
    last_name character varying(100),
    avatar_url character varying(500),
    bio text,
    is_active boolean DEFAULT true NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    email_verified boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.users OWNER TO psychicadmin;

--
-- Name: TABLE users; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.users IS 'User accounts for authentication';


--
-- Name: COLUMN users.email; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON COLUMN public.users.email IS 'User email (unique, can be null for OAuth-only users)';


--
-- Name: COLUMN users.username; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON COLUMN public.users.username IS 'User display name (unique, can be null for OAuth-only users)';


--
-- Name: COLUMN users.password_hash; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON COLUMN public.users.password_hash IS 'Bcrypt hashed password for local authentication';


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.users_id_seq OWNER TO psychicadmin;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: venues; Type: TABLE; Schema: public; Owner: psychicadmin
--

CREATE TABLE public.venues (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    address character varying(500),
    city character varying(255),
    state character varying(10),
    zipcode character varying(20),
    instagram character varying(255),
    facebook character varying(255),
    twitter character varying(255),
    youtube character varying(255),
    spotify character varying(255),
    soundcloud character varying(255),
    bandcamp character varying(255),
    website character varying(255),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.venues OWNER TO psychicadmin;

--
-- Name: TABLE venues; Type: COMMENT; Schema: public; Owner: psychicadmin
--

COMMENT ON TABLE public.venues IS 'Concert venues and locations';


--
-- Name: venues_id_seq; Type: SEQUENCE; Schema: public; Owner: psychicadmin
--

CREATE SEQUENCE public.venues_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.venues_id_seq OWNER TO psychicadmin;

--
-- Name: venues_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: psychicadmin
--

ALTER SEQUENCE public.venues_id_seq OWNED BY public.venues.id;


--
-- Name: artists id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.artists ALTER COLUMN id SET DEFAULT nextval('public.artists_id_seq'::regclass);


--
-- Name: oauth_accounts id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.oauth_accounts ALTER COLUMN id SET DEFAULT nextval('public.oauth_accounts_id_seq'::regclass);


--
-- Name: shows id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.shows ALTER COLUMN id SET DEFAULT nextval('public.shows_id_seq'::regclass);


--
-- Name: user_preferences id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.user_preferences ALTER COLUMN id SET DEFAULT nextval('public.user_preferences_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: venues id; Type: DEFAULT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.venues ALTER COLUMN id SET DEFAULT nextval('public.venues_id_seq'::regclass);


--
-- Data for Name: artists; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.artists (id, name, state, city, instagram, facebook, twitter, youtube, spotify, soundcloud, bandcamp, website, created_at, updated_at) FROM stdin;
1	Sasami	\N	\N	sasamiashworth	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
2	Where's Lucy?	\N	\N	whereslucyband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
3	Sewerbitch!	AZ	\N	sewerbitchband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
4	Droll	\N	\N	drollmusic	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
5	Bill Orcutt	\N	\N	palilaliarecords	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
6	Post Crucifixion	\N	\N	postcrucifixion	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
7	Spicy Mayo	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
8	Bad Nerves	\N	\N	badbadnerves	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
9	Alvvays	\N	\N	alvvaysband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
10	Secret Attraction	AZ	\N	secretattractionmusic	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
11	REALM	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
12	Gouge Away	\N	\N	gougeawayfl	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
13	Blood Club	\N	\N	bl00dclub	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
14	Jade Helm	AZ	\N	jade_helm_	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
15	Spiritual Cramp	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
16	Bleach	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
17	She's Green	\N	\N	shes__green	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
18	Johnny Dynamite and the Bloodsuckers	\N	\N	johnny.dynamite	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
19	Pinstock	\N	\N	pinstockband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
20	Cold Gawd	\N	\N	cldgwd	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
21	LCD Soundsystem	\N	\N	lcdsoundsystem	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
22	Fashion Club (LA)	\N	\N	fashion.club.la	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
23	URIN	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
24	Rotting Yellow	AZ	\N	rottingyellow	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
25	Rose City Band	\N	\N	rosecityband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
26	Law Abiding Citizen	AZ	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
27	Soccer Mommy	\N	\N	soccermommyband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
28	Baths	\N	\N	bathsmusic	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
29	Viagra Boys	\N	\N	viagraboys	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
30	Kochany	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
31	Tropical Fuck Storm	\N	\N	tropical_fuck_storm	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
32	Burn Victim	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
33	Abronia	\N	\N	abroniaband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
34	Sativan	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
35	Pixies	\N	\N	pixiesofficial	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
36	Bijoux Cone	\N	\N	bijouxcone	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
37	Sylvan Esso	\N	\N	sylvanesso	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
38	Vision Video	\N	\N	visionvideoband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
39	Daiistar	\N	\N	daiistarr	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
40	Lonna Kelley	AZ	\N	mslonnabee	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
41	Baller	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
42	Dummy	\N	\N	notdummy	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
43	Spellxcaster	\N	\N	spellxcasterxoxo	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
44	Winter	\N	\N	daydreamingwinter	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
45	Morphia Slow	AZ	\N	morphia.slow	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
46	The Sheaves	AZ	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
47	OSEES	\N	\N	deathgodrecords	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
48	Militarie Gun	\N	\N	militariegun	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
49	Le Mal	AZ	\N	le_mal_band	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
50	Flower Festival	AZ	\N	fflowerfestival	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
51	Cheekbone	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
52	Dune Rats	\N	\N	dunerats	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
53	high.	\N	\N	high.asfuckband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
54	L.A. Witch	\N	\N	la_witch	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
55	Treasure Mammal	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
56	Cursive	\N	\N	cursivetheband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
57	Mogwai	\N	\N	mogwaiband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
58	Jia Pet	\N	\N	jia._.pet	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
59	Hooky	\N	\N	h0o0ky	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
60	Cobarde	AZ	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
61	Eggy	\N	\N	eggymusic	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
62	Slow Pulp	\N	\N	slowpulpband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
63	JPW	AZ	\N	jasonpwoodbury	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
64	The Linda Lindas	\N	\N	thelindalindas	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
65	Amyl and The Sniffers	\N	\N	amylandthesniffers	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
66	Standing at the Back	AZ	\N	standingatthebackaz	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
67	Faetooth	\N	\N	faetooth	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
68	Iress	\N	\N	weareiress	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
69	Corbeau Hangs	AZ	\N	corbeauhangs	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
70	Artificial Go	\N	\N	artificial.go	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
71	Pile	\N	\N	pilemusic	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
72	La Luz	\N	\N	laluzband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
73	Chalcogen	\N	\N	chalcogentheband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
74	Isaac Daze	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
75	High Vis	\N	\N	highvis	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
76	Obskuros	\N	\N	obskuros	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
77	Justice	\N	\N	etjusticepourtous	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
78	Alex Okami	\N	\N	alex.okami	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
79	Mount Eerie	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
80	Yellowcake	AZ	\N	yellowcakephx	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
81	Zzzahara	\N	\N	zzzahara.wav	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
82	Michah Preite	\N	\N	mnpreite	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
83	Playboy Manbaby	AZ	\N	playboymanbaby	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
84	Glixen	AZ	\N	glixen	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
85	Youth Lagoon	\N	\N	trevorpowers	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
86	Of Montreal	\N	\N	of_montreal	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
87	Palomino	\N	\N	palomino_blond	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
88	Simian	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
89	Dumpster Abortion	\N	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
90	Prison Affair	\N	\N	prison_affair	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
91	Korine	\N	\N	korineband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
92	Chat Pile	\N	\N	chatpileband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
93	Nightosphere	\N	\N	nightospherekc	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
94	Repression	AZ	\N		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
95	Kraftwerk	\N	\N	kraftwerkofficial	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
96	Black Mountain	\N	\N	blackmountainarmy	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
97	Neu Bloom	\N	\N	neublumeband	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.735341	2025-08-03 20:23:10.735341
110	Destroyer	\N	\N	\N	\N	\N	\N	\N	\N	\N	\N	2025-09-16 19:03:19.382221	2025-09-16 19:03:19.382221
111	Jennifer Castle	\N	\N	\N	\N	\N	\N	\N	\N	\N	\N	2025-09-16 19:13:04.110334	2025-09-16 19:13:04.110334
\.


--
-- Data for Name: oauth_accounts; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.oauth_accounts (id, user_id, provider, provider_user_id, provider_email, provider_name, provider_avatar_url, access_token, refresh_token, expires_at, created_at, updated_at) FROM stdin;
\.


--
-- Data for Name: schema_migrations; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.schema_migrations (version, dirty) FROM stdin;
1	f
\.


--
-- Data for Name: show_artists; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.show_artists (show_id, artist_id, "position", set_type) FROM stdin;
1	56	0	headliner
1	71	1	opener
2	92	0	headliner
2	12	1	opener
2	93	2	opener
3	79	0	headliner
4	73	0	headliner
4	30	1	opener
4	74	2	opener
5	19	0	headliner
5	2	1	opener
5	3	2	opener
5	66	3	opener
6	87	0	headliner
6	53	1	opener
6	4	2	opener
7	27	0	headliner
8	23	0	headliner
8	80	1	opener
8	94	2	opener
8	60	3	opener
9	28	0	headliner
9	22	1	opener
10	21	0	headliner
10	9	1	opener
10	62	2	opener
11	77	0	headliner
11	37	1	opener
11	61	2	opener
12	83	0	headliner
12	52	1	opener
13	25	0	headliner
13	63	1	opener
14	67	0	headliner
14	68	1	opener
14	24	2	opener
15	16	0	headliner
15	88	1	opener
15	26	2	opener
15	32	3	opener
15	89	4	opener
15	41	5	opener
16	69	0	headliner
16	49	1	opener
16	6	2	opener
16	78	3	opener
17	33	0	headliner
17	63	1	opener
17	50	2	opener
18	84	0	headliner
19	11	0	headliner
19	34	1	opener
19	51	2	opener
20	64	0	headliner
21	81	0	headliner
21	82	1	opener
21	10	2	opener
22	84	0	headliner
22	17	1	opener
23	65	0	headliner
24	38	0	headliner
24	69	1	opener
24	49	2	opener
25	3	0	headliner
25	55	1	opener
25	7	2	opener
26	85	0	headliner
27	97	0	headliner
27	45	1	opener
27	40	2	opener
28	95	0	headliner
29	90	0	headliner
29	14	1	opener
30	96	0	headliner
31	42	0	headliner
31	46	1	opener
32	1	0	headliner
32	58	1	opener
33	57	0	headliner
34	72	0	headliner
35	42	0	headliner
36	75	0	headliner
36	48	1	opener
36	20	2	opener
37	91	0	headliner
37	18	1	opener
37	43	2	opener
38	13	0	headliner
38	76	1	opener
39	8	0	headliner
39	15	1	opener
40	54	0	headliner
40	39	1	opener
41	70	0	headliner
42	86	0	headliner
42	36	1	opener
43	35	0	headliner
44	35	0	headliner
45	31	0	headliner
45	5	1	opener
46	29	0	headliner
47	47	0	headliner
62	110	0	headliner
62	111	1	opener
\.


--
-- Data for Name: show_venues; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.show_venues (show_id, venue_id) FROM stdin;
1	6
2	9
3	18
4	15
5	5
6	11
7	12
8	4
9	17
10	14
11	14
12	6
13	7
14	13
15	4
16	7
17	15
18	17
19	2
20	8
21	7
22	6
23	3
24	9
25	2
26	17
27	11
28	10
29	9
30	7
31	11
32	16
33	3
34	7
35	17
36	16
37	7
38	16
39	6
40	7
41	1
42	18
43	3
44	3
45	7
46	3
47	6
62	16
\.


--
-- Data for Name: shows; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.shows (id, title, event_date, city, state, price, age_requirement, description, created_at, updated_at) FROM stdin;
1	Cursive, Pile at Crescent Ballroom	2025-02-19 01:30:00	Phoenix	AZ	27.00	21+	\N	2025-08-03 20:23:10.789395	2025-08-03 20:23:10.789395
2	Chat Pile, Gouge Away, Nightosphere at Nile Theater	2025-02-21 02:30:00	Mesa	AZ	25.00	All Ages	\N	2025-08-03 20:23:10.827467	2025-08-03 20:23:10.827467
3	Mount Eerie at 191 Toole	2025-02-21 03:00:00	Tucson	AZ	25.00	All Ages	\N	2025-08-03 20:23:10.847321	2025-08-03 20:23:10.847321
4	Chalcogen, Kochany, Isaac Daze at Myspace	2025-02-22 02:30:00	Phoenix	AZ	\N		\N	2025-08-03 20:23:10.868187	2025-08-03 20:23:10.868187
5	Pinstock, Where's Lucy?, Sewerbitch!, Standing At The Back at Rips	2025-02-23 03:00:00	Phoenix	AZ	\N	21+	\N	2025-08-03 20:23:10.909258	2025-08-03 20:23:10.909258
6	Palomino, High., Droll at Linger Longer Lounge	2025-02-26 03:00:00	Phoenix	AZ	\N	21+	\N	2025-08-03 20:23:10.952962	2025-08-03 20:23:10.952962
7	Soccer Mommy at Rialto Theatre	2025-02-26 03:00:00	Tucson	AZ	30.00	All Ages	\N	2025-08-03 20:23:10.977846	2025-08-03 20:23:10.977846
8	Urin, Yellowcake, Repression, Cobarde at Palo Verde Lounge	2025-02-26 03:00:00	Phoenix	AZ	10.00	21+	\N	2025-08-03 20:23:10.99461	2025-08-03 20:23:10.99461
9	Baths, Fashion Club at Club Congress	2025-03-08 02:00:00	Tucson	AZ	20.00	All Ages	\N	2025-08-03 20:23:11.035595	2025-08-03 20:23:11.035595
10	Lcd Soundsystem, Alvvays, Slow Pulp at M3f Fest	2025-03-07 20:00:00	Phoenix	AZ	110.00	21+	\N	2025-08-03 20:23:11.069282	2025-08-03 20:23:11.069282
11	Justice, Sylvan Esso, Eggy at M3f Fest	2025-03-08 20:00:00	Phoenix	AZ	110.00	21+	\N	2025-08-03 20:23:11.097417	2025-08-03 20:23:11.097417
12	Playboy Manbaby, Dune Rats at Crescent Ballroom	2025-03-09 02:00:00	Phoenix	AZ	22.00	16+	\N	2025-08-03 20:23:11.128758	2025-08-03 20:23:11.128758
13	Rose City Band, Jpw at Valley Bar	2025-03-11 01:30:00	Phoenix	AZ	16.00	21+	\N	2025-08-03 20:23:11.153368	2025-08-03 20:23:11.153368
14	Faetooth, Iress, Rotting Yellow at The Underground	2025-03-21 01:30:00	Mesa	AZ	18.00	All Ages	\N	2025-08-03 20:23:11.174311	2025-08-03 20:23:11.174311
15	Bleach, Simian, Law Abiding Citizen, Burn Victim, Dumpster Abortion, Baller at Palo Verde Lounge	2025-03-22 03:00:00	Phoenix	AZ	\N	21+	\N	2025-08-03 20:23:11.204312	2025-08-03 20:23:11.204312
16	Corbeau Hangs, Le Mal, Post Crucifixion, Alex Okami at Valley Bar	2025-03-23 02:00:00	Phoenix	AZ	15.00	16+	\N	2025-08-03 20:23:11.246668	2025-08-03 20:23:11.246668
17	Abronia, Jpw, Flower Festival at Myspace	2025-03-26 02:00:00	Phoenix	AZ	10.00	21+	\N	2025-08-03 20:23:11.278145	2025-08-03 20:23:11.278145
18	Glixen at Club Congress	2025-03-30 02:00:00	Tucson	AZ	18.00	All Ages	\N	2025-08-03 20:23:11.30507	2025-08-03 20:23:11.30507
19	Realm, Sativan, Cheekbone at Gracie's Tax Bar	2025-03-31 01:30:00	Phoenix	AZ	\N	21+	\N	2025-08-03 20:23:11.322511	2025-08-03 20:23:11.322511
20	The Linda Lindas at Walter Studios	2025-04-01 02:00:00	Phoenix	AZ	25.00	16+	\N	2025-08-03 20:23:11.348888	2025-08-03 20:23:11.348888
21	Zzzahara, Michah Preite, Secret Attraction at Valley Bar	2025-04-04 02:30:00	Phoenix	AZ	20.00	16+	\N	2025-08-03 20:23:11.365414	2025-08-03 20:23:11.365414
22	Glixen, She's Green at Crescent Ballroom	2025-04-06 02:00:00	Phoenix	AZ	18.00	16+	\N	2025-08-03 20:23:11.388769	2025-08-03 20:23:11.388769
23	Amyl And The Sniffers at The Van Buren	2025-04-11 03:00:00	Phoenix	AZ	48.00	13+	\N	2025-08-03 20:23:11.409715	2025-08-03 20:23:11.409715
24	Vision Video, Corbeau Hangs, Le Mal at Nile Theater	2025-04-11 02:00:00	Mesa	AZ	20.00	All Ages	\N	2025-08-03 20:23:11.426867	2025-08-03 20:23:11.426867
25	Sewerbitch!, Treasure Mammal, Spicy Mayo at Gracie's Tax Bar	2025-04-12 01:30:00	Phoenix	AZ	\N	21+	\N	2025-08-03 20:23:11.453459	2025-08-03 20:23:11.453459
26	Youth Lagoon at Club Congress	2025-04-12 02:00:00	Tucson	AZ	25.00	All Ages	\N	2025-08-03 20:23:11.480944	2025-08-03 20:23:11.480944
27	Neu Bloom, Morphia Slow, Lonna Kelley at Linger Longer Lounge	2025-04-13 02:00:00	Phoenix	AZ	12.00	21+	\N	2025-08-03 20:23:11.500249	2025-08-03 20:23:11.500249
28	Kraftwerk at Orpheum Theater	2025-04-15 03:00:00	Pheonix	AZ	101.00	21+	\N	2025-08-03 20:23:11.528206	2025-08-03 20:23:11.528206
29	Prison Affair, Jade Helm at Nile Theater	2025-04-19 02:00:00	Phoenix	AZ	22.00	21+	\N	2025-08-03 20:23:11.546557	2025-08-03 20:23:11.546557
30	Black Mountain at Valley Bar	2025-04-22 02:00:00	Phoenix	AZ	20.00	21+	\N	2025-08-03 20:23:11.56882	2025-08-03 20:23:11.56882
31	Dummy, The Sheaves at Linger Longer Lounge	2025-04-23 02:30:00	Phoenix	AZ	15.00	21+	\N	2025-08-03 20:23:11.586861	2025-08-03 20:23:11.586861
32	Sasami, Jia Pet at The Rebel Lounge	2025-04-23 03:00:00	Phoenix	AZ	22.00	All Ages	\N	2025-08-03 20:23:11.607918	2025-08-03 20:23:11.607918
33	Mogwai at The Van Buren	2025-05-01 03:00:00	Phoenix	AZ	43.00	13+	\N	2025-08-03 20:23:11.630755	2025-08-03 20:23:11.630755
34	La Luz at Valley Bar	2025-05-02 03:00:00	Phoenix	AZ	22.00	16+	\N	2025-08-03 20:23:11.651534	2025-08-03 20:23:11.651534
35	Dummy at Club Congress	2025-05-03 02:00:00	Phoenix	AZ	18.00	All Ages	\N	2025-08-03 20:23:11.672438	2025-08-03 20:23:11.672438
36	High Vis, Militarie Gun, Cold Gawd at The Rebel Lounge	2025-05-11 03:00:00	Phoenix	AZ	30.00	All Ages	\N	2025-08-03 20:23:11.691465	2025-08-03 20:23:11.691465
37	Korine, Johnny Dynamite And The Bloodsuckers, Spellxcaster at Valley Bar	2025-05-11 02:00:00	Phoenix	AZ	17.00	21+	\N	2025-08-03 20:23:11.732078	2025-08-03 20:23:11.732078
38	Blood Club, Obskuros at The Rebel Lounge	2025-05-16 03:00:00	Phoenix	AZ	18.00	21+	\N	2025-08-03 20:23:11.761819	2025-08-03 20:23:11.761819
39	Bad Nerves, Spiritual Cramp at Crescent Ballroom	2025-05-20 02:00:00	Phoenix	AZ	25.00	16+	\N	2025-08-03 20:23:11.78676	2025-08-03 20:23:11.78676
40	L.a. Witch, Daiistar at Valley Bar	2025-05-22 02:30:00	Phoenix	AZ	18.00	21+	\N	2025-08-03 20:23:11.807603	2025-08-03 20:23:11.807603
41	Artificial Go at Tba	2025-06-06 02:00:00	Tucson	AZ	\N	21+	\N	2025-08-03 20:23:11.832681	2025-08-03 20:23:11.832681
42	Of Montreal, Bijoux Cone at 191 Toole	2025-06-08 03:00:00	Tucson	AZ	30.00	21 & Over	\N	2025-08-03 20:23:11.849377	2025-08-03 20:23:11.849377
43	Pixies at The Van Buren	2025-06-17 03:00:00	Phoenix	AZ	75.50	13+	\N	2025-08-03 20:23:11.869519	2025-08-03 20:23:11.869519
44	Pixies at The Van Buren	2025-06-18 03:00:00	Phoenix	AZ	75.50	13+	\N	2025-08-03 20:23:11.885693	2025-08-03 20:23:11.885693
45	Tropical Fuck Storm, Bill Orcutt at Valley Bar	2025-07-03 02:00:00	Phoenix	AZ	19.00	21+	\N	2025-08-03 20:23:11.905484	2025-08-03 20:23:11.905484
46	Viagra Boys at The Van Buren	2025-10-26 03:00:00	Phoenix	AZ	41.75	13+	\N	2025-08-03 20:23:11.933155	2025-08-03 20:23:11.933155
47	Osees at Crescent Ballroom	2025-11-06 02:00:00	Phoenix	AZ	32.00	21+	\N	2025-08-03 20:23:11.951579	2025-08-03 20:23:11.951579
62	Dan's Boogie Tour	2025-09-26 01:00:00	Phoenix	Arizona	\N	21+		2025-09-16 19:13:04.097812	2025-09-16 19:13:04.097812
\.


--
-- Data for Name: user_preferences; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.user_preferences (id, user_id, notification_email, notification_push, theme, timezone, language, created_at, updated_at) FROM stdin;
1	1	t	f	light	UTC	en	2025-09-07 21:39:01.583623	2025-09-07 21:39:01.583623
2	2	t	f	light	UTC	en	2025-09-07 22:34:10.221979	2025-09-07 22:34:10.221979
3	3	t	f	light	UTC	en	2025-09-11 14:31:36.980987	2025-09-11 14:31:36.980987
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.users (id, email, username, password_hash, first_name, last_name, avatar_url, bio, is_active, is_admin, email_verified, created_at, updated_at) FROM stdin;
2	admin@admin.com	\N	$2a$10$.2f6JmkrcHAl4HjKRpe5j.3L7BfYr.fSXju127jXVJkclKJuiC2/.			\N	\N	t	f	f	2025-09-07 22:34:10.208104	2025-09-07 22:34:10.208104
1	asdf@admin.com	adminadmin	$2a$10$VNhbS4JlpNku3AeK6NEq9.ZwhaZy56iBDZDxvzyM9rCfRuvUYHteK			\N	\N	t	f	f	2025-09-07 21:39:01.565349	2025-09-07 21:39:01.565349
3	bob@true.com	\N	$2a$10$X5PadajBIKy8SaTqgs9G6OkIb.LmJW2J07uKoQ02ZncdrW94jN89u			\N	\N	t	f	f	2025-09-11 14:31:36.963622	2025-09-11 14:31:36.963622
\.


--
-- Data for Name: venues; Type: TABLE DATA; Schema: public; Owner: psychicadmin
--

COPY public.venues (id, name, address, city, state, zipcode, instagram, facebook, twitter, youtube, spotify, soundcloud, bandcamp, website, created_at, updated_at) FROM stdin;
1	TBA		Tucson	AZ			\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
2	Gracie's Tax Bar		Phoenix	AZ			\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
3	The Van Buren	401 W Van Buren St	Phoenix	AZ	85003	thevanburenphx	\N	\N	\N	\N	\N	\N	https://www.thevanburenphx.com	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
4	Palo Verde Lounge	1015 W Broadway Rd	Tempe	AZ	85282	verde_palo	\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
5	Rips	3045 N 16th St	Phoenix	AZ	85016		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
6	Crescent Ballroom	308 N 2nd Ave	Phoenix	AZ	85003	crescentphx	\N	\N	\N	\N	\N	\N	https://www.crescentphx.com	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
7	Valley Bar	130 N Central Ave	Phoenix	AZ	85004	valleybarphx	\N	\N	\N	\N	\N	\N	https://www.valleybarphx.com	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
8	Walter Studios	747 W Roosevelt St	Phoenix	AZ	85007	walterstudiosphx	\N	\N	\N	\N	\N	\N	https://walterstudios.com/	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
9	Nile Theater	105 W Main St	Mesa	AZ	85201		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
10	Orpheum Theater	203 W Adams St	Phoenix	AZ	85003		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
11	Linger Longer Lounge	6522 N 16th St	Phoenix	AZ	85016	lingerlongeraz	\N	\N	\N	\N	\N	\N	https://www.lingerlongerlounge.com/	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
12	Rialto Theatre	318 E Congress St	Tucson	AZ	85701		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
13	The Underground		Mesa	AZ			\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
14	M3F Fest		Phoenix	AZ			\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
15	Myspace	120 E Roosevelt St	Phoenix	AZ	85004		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
16	The Rebel Lounge	2303 E Indian School Rd	Phoenix	AZ	85016	therebelphx	\N	\N	\N	\N	\N	\N	https://www.therebellounge.com	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
17	Club Congress	311 E Congress St	Tucson	AZ	85701	hotelcongress	\N	\N	\N	\N	\N	\N	https://hotelcongress.com/music	2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
18	191 Toole	191 E Toole Ave	Tucson	AZ	85701		\N	\N	\N	\N	\N	\N		2025-08-03 20:23:10.717282	2025-08-03 20:23:10.717282
\.


--
-- Name: artists_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.artists_id_seq', 112, true);


--
-- Name: oauth_accounts_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.oauth_accounts_id_seq', 1, false);


--
-- Name: shows_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.shows_id_seq', 64, true);


--
-- Name: user_preferences_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.user_preferences_id_seq', 3, true);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.users_id_seq', 3, true);


--
-- Name: venues_id_seq; Type: SEQUENCE SET; Schema: public; Owner: psychicadmin
--

SELECT pg_catalog.setval('public.venues_id_seq', 27, true);


--
-- Name: artists artists_name_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.artists
    ADD CONSTRAINT artists_name_key UNIQUE (name);


--
-- Name: artists artists_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.artists
    ADD CONSTRAINT artists_pkey PRIMARY KEY (id);


--
-- Name: oauth_accounts oauth_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_pkey PRIMARY KEY (id);


--
-- Name: oauth_accounts oauth_accounts_provider_provider_user_id_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_provider_provider_user_id_key UNIQUE (provider, provider_user_id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: show_artists show_artists_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_artists
    ADD CONSTRAINT show_artists_pkey PRIMARY KEY (show_id, artist_id);


--
-- Name: show_venues show_venues_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_venues
    ADD CONSTRAINT show_venues_pkey PRIMARY KEY (show_id, venue_id);


--
-- Name: shows shows_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.shows
    ADD CONSTRAINT shows_pkey PRIMARY KEY (id);


--
-- Name: user_preferences user_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_pkey PRIMARY KEY (id);


--
-- Name: user_preferences user_preferences_user_id_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_key UNIQUE (user_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: venues venues_name_key; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.venues
    ADD CONSTRAINT venues_name_key UNIQUE (name);


--
-- Name: venues venues_pkey; Type: CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.venues
    ADD CONSTRAINT venues_pkey PRIMARY KEY (id);


--
-- Name: idx_artists_name; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_artists_name ON public.artists USING btree (name);


--
-- Name: idx_oauth_accounts_provider; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_oauth_accounts_provider ON public.oauth_accounts USING btree (provider);


--
-- Name: idx_oauth_accounts_provider_email; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_oauth_accounts_provider_email ON public.oauth_accounts USING btree (provider_email);


--
-- Name: idx_oauth_accounts_provider_user_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_oauth_accounts_provider_user_id ON public.oauth_accounts USING btree (provider_user_id);


--
-- Name: idx_oauth_accounts_user_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_oauth_accounts_user_id ON public.oauth_accounts USING btree (user_id);


--
-- Name: idx_show_artists_artist_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_show_artists_artist_id ON public.show_artists USING btree (artist_id);


--
-- Name: idx_show_artists_position; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_show_artists_position ON public.show_artists USING btree (show_id, "position");


--
-- Name: idx_show_artists_show_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_show_artists_show_id ON public.show_artists USING btree (show_id);


--
-- Name: idx_show_venues_show_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_show_venues_show_id ON public.show_venues USING btree (show_id);


--
-- Name: idx_show_venues_venue_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_show_venues_venue_id ON public.show_venues USING btree (venue_id);


--
-- Name: idx_shows_city; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_shows_city ON public.shows USING btree (city);


--
-- Name: idx_shows_event_date; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_shows_event_date ON public.shows USING btree (event_date);


--
-- Name: idx_user_preferences_user_id; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_user_preferences_user_id ON public.user_preferences USING btree (user_id);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_email_verified; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_users_email_verified ON public.users USING btree (email_verified);


--
-- Name: idx_users_is_active; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_users_is_active ON public.users USING btree (is_active);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_users_username ON public.users USING btree (username);


--
-- Name: idx_venues_name; Type: INDEX; Schema: public; Owner: psychicadmin
--

CREATE INDEX idx_venues_name ON public.venues USING btree (name);


--
-- Name: oauth_accounts oauth_accounts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: show_artists show_artists_artist_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_artists
    ADD CONSTRAINT show_artists_artist_id_fkey FOREIGN KEY (artist_id) REFERENCES public.artists(id) ON DELETE CASCADE;


--
-- Name: show_artists show_artists_show_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_artists
    ADD CONSTRAINT show_artists_show_id_fkey FOREIGN KEY (show_id) REFERENCES public.shows(id) ON DELETE CASCADE;


--
-- Name: show_venues show_venues_show_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_venues
    ADD CONSTRAINT show_venues_show_id_fkey FOREIGN KEY (show_id) REFERENCES public.shows(id) ON DELETE CASCADE;


--
-- Name: show_venues show_venues_venue_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.show_venues
    ADD CONSTRAINT show_venues_venue_id_fkey FOREIGN KEY (venue_id) REFERENCES public.venues(id) ON DELETE CASCADE;


--
-- Name: user_preferences user_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: psychicadmin
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict afosmmn5QaWc9wj5vcZvIBBuTPaKzpPrSUtLGskEnW08dEjGvVysDictcQYJgJu

