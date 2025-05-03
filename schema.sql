--
-- PostgreSQL database dump
--

-- Dumped from database version 17.4
-- Dumped by pg_dump version 17.4

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
-- Name: guilds; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.guilds (
    guild_id character varying(20) NOT NULL,
    channel_id character varying(20),
    role_id character varying(20),
    logs_channel_id character varying(20)
);


ALTER TABLE public.guilds OWNER TO postgres;

--
-- Name: verifyuserlogs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.verifyuserlogs (
    id integer NOT NULL,
    request_id uuid,
    user_id character varying(20),
    fingerprint character varying(40),
    ip_score real,
    guild_id character varying(20) NOT NULL,
    passed boolean
);


ALTER TABLE public.verifyuserlogs OWNER TO postgres;

--
-- Name: verifyuserlogs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.verifyuserlogs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.verifyuserlogs_id_seq OWNER TO postgres;

--
-- Name: verifyuserlogs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.verifyuserlogs_id_seq OWNED BY public.verifyuserlogs.id;


--
-- Name: verifyusers; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.verifyusers (
    request_id uuid NOT NULL,
    user_id character varying(20),
    request_time bigint,
    verify_message_id character varying(20),
    guild_id character varying(20) NOT NULL,
    verify_channel_id character varying(20)
);


ALTER TABLE public.verifyusers OWNER TO postgres;

--
-- Name: verifyuserlogs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.verifyuserlogs ALTER COLUMN id SET DEFAULT nextval('public.verifyuserlogs_id_seq'::regclass);


--
-- Name: guilds guilds_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.guilds
    ADD CONSTRAINT guilds_pkey PRIMARY KEY (guild_id);


--
-- Name: verifyuserlogs verifyuserlogs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.verifyuserlogs
    ADD CONSTRAINT verifyuserlogs_pkey PRIMARY KEY (id);


--
-- Name: verifyusers verifyusers_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.verifyusers
    ADD CONSTRAINT verifyusers_pkey PRIMARY KEY (request_id);


--
-- Name: verifyusers fk_guild_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.verifyusers
    ADD CONSTRAINT fk_guild_id FOREIGN KEY (guild_id) REFERENCES public.guilds(guild_id);


--
-- Name: verifyuserlogs fk_guild_id; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.verifyuserlogs
    ADD CONSTRAINT fk_guild_id FOREIGN KEY (guild_id) REFERENCES public.guilds(guild_id);


--
-- PostgreSQL database dump complete
--

