-- Copyright 2018 Google Inc. All Rights Reserved.
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
-- 
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

-- Cloud Spanner Schema for Ticketshop Demo ;

CREATE TABLE MultiEvent (
    MultiEventId    STRING(32) NOT NULL,
    Name            STRING(256) NOT NULL
) PRIMARY KEY (MultiEventId);

CREATE TABLE MultiEventCounter (
    MultiEventId STRING(32) NOT NULL,
    CustomerRegion  STRING(2) NOT NULL,
    Shard INT64 NOT NULL,
    Sold INT64 NOT NULL,
    Revenue FLOAT64 NOT NULL
) PRIMARY KEY(MultiEventId, CustomerRegion, Shard),
INTERLEAVE IN PARENT MultiEvent ON DELETE CASCADE;

CREATE TABLE Venue (
    VenueId         STRING(32) NOT NULL,
    VenueName       STRING(256) NOT NULL,
    CountryCode     STRING(2) NOT NULL
) PRIMARY KEY (VenueId);

CREATE TABLE VenueInfo (
    VenueId         STRING(32) NOT NULL,
    Data            BYTES(MAX)
) PRIMARY KEY (VenueId);

CREATE TABLE SeatingCategory (
    VenueId         STRING(32) NOT NULL,
    CategoryId      STRING(32) NOT NULL,
    VenueConfig     STRING(32) NOT NULL,
    SeatingConfig   STRING(32) NOT NULL,
    CategoryName    STRING(128) NOT NULL,
    Seats           INT64 NOT NULL
) PRIMARY KEY (VenueId, CategoryId, VenueConfig),
INTERLEAVE IN PARENT Venue ON DELETE CASCADE;
CREATE INDEX SeatingCategoryById ON SeatingCategory(CategoryId, VenueId) STORING (CategoryName, Seats);

CREATE TABLE Account (
    AccountId       STRING(32) NOT NULL,
    Name            STRING(256) NOT NULL,
    EMail           STRING(256) NOT NULL,
    CountryCode     STRING(2) NOT NULL,
    Tickets         ARRAY<STRING(32)>
) PRIMARY KEY (AccountId, EMail);

CREATE TABLE Event (
    EventId         STRING(32) NOT NULL,
    MultiEventId    STRING(32) NOT NULL,
    EventName       STRING(256) NOT NULL,
    EventDate       TIMESTAMP NOT NULL,
    SaleDate        TIMESTAMP NOT NULL
) PRIMARY KEY (EventId);
CREATE INDEX EventByMultiEventId ON Event(MultiEventId, SaleDate) STORING(EventName, EventDate);
CREATE INDEX EventByDate ON Event (EventDate) STORING (MultiEventId, SaleDate, EventName);

CREATE TABLE EventInfo (
    EventId         STRING(32) NOT NULL,
    Data            BYTES(MAX)
) PRIMARY KEY (EventId);

CREATE TABLE TicketCategory (
    EventId         STRING(32) NOT NULL,
    CategoryId      STRING(32) NOT NULL,
    Price           FLOAT64 NOT NULL
) PRIMARY KEY (EventId, CategoryId),
INTERLEAVE IN PARENT Event ON DELETE CASCADE;

CREATE TABLE EventCategoryCounter (
    EventId STRING(32) NOT NULL,
    CategoryId STRING(32) NOT NULL,
    CustomerRegion  STRING(2) NOT NULL,
    Shard INT64 NOT NULL,
    Sold INT64 NOT NULL,
    Revenue FLOAT64 NOT NULL
) PRIMARY KEY(EventId, CategoryId, CustomerRegion, Shard),
INTERLEAVE IN PARENT Event ON DELETE CASCADE;

CREATE TABLE Ticket (
    TicketId        STRING(32) NOT NULL,
    CategoryId      STRING(32) NOT NULL,
    EventId         STRING(32) NOT NULL,
    AccountId       STRING(32),
    SeqNo           INT64 NOT NULL,
    PurchaseDate    TIMESTAMP,
    Available       BOOL
) PRIMARY KEY (EventId, CategoryId, TicketId);

CREATE INDEX TicketsSold ON Ticket (SeqNo, Available);
CREATE NULL_FILTERED INDEX TicketsAvailable ON Ticket(EventId, CategoryId, SeqNo, Available, TicketId);

CREATE TABLE SoldTicketsCounter (
    EventId         STRING(32) NOT NULL,
    CategoryId      STRING(32) NOT NULL,
    Shard           INT64 NOT NULL,
    Sold            INT64 NOT NULL
) PRIMARY KEY(EventId, CategoryId, Shard);

CREATE TABLE TicketInfo (
    TicketId        STRING(32) NOT NULL,
    Data            BYTES(MAX)
) PRIMARY KEY (TicketId);

CREATE TABLE RegionCounter (
    Region          STRING(2) NOT NULL,
    CustomerRegion  STRING(2) NOT NULL,
    Shard           INT64 NOT NULL,
    Sold            INT64 NOT NULL,
    Revenue         FLOAT64 NOT NULL
) PRIMARY KEY(Region, CustomerRegion, Shard);

CREATE TABLE CountryCounter (
    CountryCode     STRING(2) NOT NULL,
    CustomerRegion  STRING(2) NOT NULL,
    Shard           INT64 NOT NULL,
    Sold            INT64 NOT NULL,
    Revenue         FLOAT64 NOT NULL
) PRIMARY KEY(CountryCode, CustomerRegion, Shard);
