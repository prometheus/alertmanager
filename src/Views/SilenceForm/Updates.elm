module Views.SilenceForm.Updates exposing (update)

import Views.SilenceForm.Types exposing (SilenceFormMsg(..))
import Types exposing (Model, Msg(MsgForSilenceForm, NewUrl, PreviewSilence))
import Utils.Types exposing (ApiResponse(Success, Loading, Failure))
import Utils.List
import Utils.Date
import Silences.Api
import Silences.Types exposing (nullMatcher, nullSilence)
import Task
import Time


update : SilenceFormMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        AddMatcher silence ->
            -- TODO: If a user adds two blank matchers and attempts to update
            -- one, both are updated because they are identical. Maybe add a
            -- unique identifier on creation so this doesn't happen.
            ( { model | silence = Success { silence | matchers = silence.matchers ++ [ nullMatcher ] } }, Cmd.none )

        CreateSilence silence ->
            ( { model | silence = Loading }, Silences.Api.create silence (SilenceCreate >> MsgForSilenceForm) )

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
                ( { model | silence = Success { silence | startsAt = startsAt, duration = duration } }, Cmd.none )

        UpdateEndsAt silence time ->
            let
                endsAt =
                    Utils.Date.timeFromString time

                duration =
                    Maybe.map2 (-) endsAt.t silence.startsAt.t
                        |> Maybe.map Utils.Date.duration
                        |> Maybe.withDefault silence.duration
            in
                ( { model | silence = Success { silence | endsAt = endsAt, duration = duration } }, Cmd.none )

        UpdateDuration silence time ->
            let
                duration =
                    Utils.Date.durationFromString time

                endsAt =
                    Maybe.map2 (+) silence.startsAt.t duration.d
                        |> Maybe.map Utils.Date.fromTime
                        |> Maybe.withDefault silence.endsAt
            in
                ( { model | silence = Success { silence | duration = duration, endsAt = endsAt } }, Cmd.none )

        UpdateCreatedBy silence by ->
            ( { model | silence = Success { silence | createdBy = by } }, Cmd.none )

        SilenceCreate silence ->
            case silence of
                Success id ->
                    ( { model | silence = Loading }, Task.perform identity (Task.succeed <| NewUrl ("/#/silence/" ++ id)) )

                Failure err ->
                    ( { model | silence = Failure err }, Task.perform identity (Task.succeed <| NewUrl "/#/silence") )

                Loading ->
                    ( { model | silence = Loading }, Task.perform identity (Task.succeed <| NewUrl "/#/silence") )

        UpdateComment silence comment ->
            ( { model | silence = Success { silence | comment = comment } }, Cmd.none )

        DeleteMatcher silence matcher ->
            let
                -- TODO: This removes all empty matchers. Maybe just remove the
                -- one that was clicked.
                newSil =
                    { silence | matchers = (List.filter (\x -> x /= matcher) silence.matchers) }
            in
                ( { model | silence = Success newSil }, Cmd.none )

        UpdateMatcherName silence matcher name ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | name = name } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        UpdateMatcherValue silence matcher value ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | value = value } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        UpdateMatcherRegex silence matcher bool ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | isRegex = bool } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        NewDefaultTimeRange time ->
            let
                startsAt =
                    Utils.Date.fromTime time

                duration =
                    Utils.Date.duration (2 * Time.hour)

                endsAt =
                    Utils.Date.fromTime (time + 2 * Time.hour)

                sil =
                    case model.silence of
                        Success s ->
                            s

                        _ ->
                            nullSilence
            in
                ( { model | silence = Success { sil | startsAt = startsAt, duration = duration, endsAt = endsAt } }, Cmd.none )

        FetchSilence silenceId ->
            ( { model | silence = model.silence }, Silences.Api.getSilence silenceId (SilenceFetch >> MsgForSilenceForm) )

        NewSilence ->
            ( { model | silence = model.silence }, Cmd.map MsgForSilenceForm (Task.perform NewDefaultTimeRange Time.now) )

        SilenceFetch sil ->
            let
                cmd =
                    case sil of
                        Success sil ->
                            Task.perform identity (Task.succeed (PreviewSilence sil))

                        _ ->
                            Cmd.none
            in
                ( { model | silence = sil }, cmd )
