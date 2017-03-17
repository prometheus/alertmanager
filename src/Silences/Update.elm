module Silences.Update exposing (..)

import Silences.Api as Api
import Silences.Types exposing (..)
import Task
import Utils.Types exposing (ApiData, ApiResponse(..), Filter, Matchers)
import Utils.Types as Types exposing (ApiData, ApiResponse(..), Time, Filter, Matchers)
import Time
import Utils.Date
import Utils.List
import Utils.Filter exposing (generateQueryString)


update : SilencesMsg -> ApiData (List Silence) -> ApiData Silence -> Filter -> ( ApiData (List Silence), ApiData Silence, Cmd Msg )
update msg silences silence filter =
    case msg of
        SilencesFetch sils ->
            ( sils, silence, Cmd.map ForParent (Task.perform UpdateCurrentTime Time.now) )

        SilenceFetch sil ->
            let
                cmd =
                    case sil of
                        Success sil ->
                            Task.perform ForParent <| Task.succeed (PreviewSilence sil)

                        _ ->
                            Cmd.none
            in
                ( silences, sil, cmd )

        FetchSilences ->
            ( silences, silence, Api.getSilences filter )

        FetchSilence id ->
            ( silences, silence, Api.getSilence id )

        NewSilence ->
            ( silences, silence, Cmd.map ForSelf (Task.perform NewDefaultTimeRange Time.now) )

        CreateSilence silence ->
            ( silences, Loading, Api.create silence )

        DestroySilence silence ->
            ( silences, Loading, Api.destroy silence )

        SilenceCreate silence ->
            case silence of
                Success id ->
                    ( silences, Loading, generateParentMsg <| NewUrl ("/#/silences/" ++ id) )

                Failure err ->
                    ( silences, Failure err, generateParentMsg <| NewUrl "/#/silences" )

                Loading ->
                    ( silences, Loading, generateParentMsg <| NewUrl "/#/silences" )

        SilenceDestroy silence ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Add DELETE to accepted CORS methods in alertmanager
            -- TODO: Check why POST isn't there but is accepted
            let
                res =
                    case silence of
                        Failure err ->
                            Failure err

                        _ ->
                            Loading
            in
                ( silences, res, generateParentMsg <| NewUrl "/#/silences" )

        UpdateStartsAt silence time ->
            -- TODO:
            -- Update silence to hold datetime as string, on each pass through
            -- here update an error message "this is invalid", but let them put
            -- it in anyway.
            let
                startsAt =
                    Utils.Date.timeFromString time

                duration =
                    Maybe.map2 (-) silence.endsAt.t startsAt.t
                        |> Maybe.map Utils.Date.duration
                        |> Maybe.withDefault silence.duration
            in
                ( silences, Success { silence | startsAt = startsAt, duration = duration }, Cmd.none )

        UpdateEndsAt silence time ->
            let
                endsAt =
                    Utils.Date.timeFromString time

                duration =
                    Maybe.map2 (-) endsAt.t silence.startsAt.t
                        |> Maybe.map Utils.Date.duration
                        |> Maybe.withDefault silence.duration
            in
                ( silences, Success { silence | endsAt = endsAt, duration = duration }, Cmd.none )

        UpdateDuration silence time ->
            let
                duration =
                    Utils.Date.durationFromString time

                endsAt =
                    Maybe.map2 (+) silence.startsAt.t duration.d
                        |> Maybe.map Utils.Date.fromTime
                        |> Maybe.withDefault silence.endsAt
            in
                ( silences, Success { silence | duration = duration, endsAt = endsAt }, Cmd.none )

        UpdateCreatedBy silence by ->
            ( silences, Success { silence | createdBy = by }, Cmd.none )

        UpdateComment silence comment ->
            ( silences, Success { silence | comment = comment }, Cmd.none )

        AddMatcher silence ->
            -- TODO: If a user adds two blank matchers and attempts to update
            -- one, both are updated because they are identical. Maybe add a
            -- unique identifier on creation so this doesn't happen.
            ( silences, Success { silence | matchers = silence.matchers ++ [ nullMatcher ] }, Cmd.none )

        DeleteMatcher silence matcher ->
            let
                -- TODO: This removes all empty matchers. Maybe just remove the
                -- one that was clicked.
                newSil =
                    { silence | matchers = (List.filter (\x -> x /= matcher) silence.matchers) }
            in
                ( silences, Success newSil, Cmd.none )

        UpdateMatcherName silence matcher name ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | name = name } silence.matchers
            in
                ( silences, Success { silence | matchers = matchers }, Cmd.none )

        UpdateMatcherValue silence matcher value ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | value = value } silence.matchers
            in
                ( silences, Success { silence | matchers = matchers }, Cmd.none )

        UpdateMatcherRegex silence matcher bool ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | isRegex = bool } silence.matchers
            in
                ( silences, Success { silence | matchers = matchers }, Cmd.none )

        NewDefaultTimeRange time ->
            let
                startsAt =
                    Utils.Date.fromTime time

                duration =
                    Utils.Date.duration (2 * Time.hour)

                endsAt =
                    Utils.Date.fromTime (time + 2 * Time.hour)

                sil =
                    case silence of
                        Success s ->
                            s

                        _ ->
                            nullSilence
            in
                ( silences, Success { sil | startsAt = startsAt, duration = duration, endsAt = endsAt }, Cmd.none )

        FilterSilences ->
            let
                url =
                    "/#/silences" ++ generateQueryString filter

                cmds =
                    Cmd.batch [ generateParentMsg (NewUrl url) ]
            in
                ( silences, silence, generateParentMsg (NewUrl url) )

        Noop ->
            ( silences, silence, Cmd.none )


generateParentMsg : OutMsg -> Cmd Msg
generateParentMsg outMsg =
    Task.perform ForParent (Task.succeed outMsg)


urlUpdate : Route -> ( SilencesMsg, Filter )
urlUpdate route =
    let
        msg =
            case route of
                ShowSilences _ ->
                    FetchSilences

                ShowSilence uuid ->
                    FetchSilence uuid

                ShowNewSilence ->
                    NewSilence

                ShowEditSilence uuid ->
                    FetchSilence uuid
    in
        ( msg, updateFilter route )


updateFilter : Route -> Filter
updateFilter route =
    case route of
        ShowSilences maybeFilter ->
            { receiver = Nothing
            , showSilenced = Nothing
            , text = maybeFilter
            }

        _ ->
            { receiver = Nothing
            , showSilenced = Nothing
            , text = Nothing
            }
