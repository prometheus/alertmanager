module Views.SilenceForm.Updates exposing (update)

import Alerts.Api
import Silences.Api
import Silences.Types exposing (nullMatcher, nullSilence)
import Task
import Time
import Navigation
import Types exposing (Msg(MsgForSilenceForm))
import Utils.Date
import Utils.List
import Utils.Types exposing (ApiResponse(..))
import Utils.Filter exposing (nullFilter)
import Views.SilenceForm.Types
    exposing
        ( Model
        , SilenceForm
        , SilenceFormMsg(..)
        , SilenceFormFieldMsg(..)
        , fromMatchersAndTime
        , fromSilence
        , toSilence
        )


updateForm : SilenceFormFieldMsg -> SilenceForm -> SilenceForm
updateForm msg form =
    case msg of
        AddMatcher ->
            { form | matchers = form.matchers ++ [ nullMatcher ] }

        UpdateStartsAt time ->
            -- TODO:
            -- Update silence to hold datetime as string, on each pass through
            -- here update an error message "this is invalid", but let them put
            -- it in anyway.
            let
                startsAt =
                    Utils.Date.timeFromString time

                endsAt =
                    Utils.Date.timeFromString form.endsAt

                duration =
                    Maybe.map2 (-) endsAt startsAt
                        |> Maybe.map Utils.Date.durationFormat
                        |> Maybe.withDefault ""
            in
                { form | startsAt = time, duration = duration }

        UpdateEndsAt time ->
            let
                startsAt =
                    Utils.Date.timeFromString form.startsAt

                endsAt =
                    Utils.Date.timeFromString time

                duration =
                    Maybe.map2 (-) endsAt startsAt
                        |> Maybe.map Utils.Date.durationFormat
                        |> Maybe.withDefault ""
            in
                { form | endsAt = time, duration = duration }

        UpdateDuration time ->
            let
                startsAt =
                    Utils.Date.timeFromString form.startsAt

                duration =
                    Utils.Date.parseDuration time

                endsAt =
                    Maybe.map2 (+) startsAt duration
                        |> Maybe.map Utils.Date.timeToString
                        |> Maybe.withDefault form.endsAt
            in
                { form | endsAt = endsAt, duration = time }

        UpdateCreatedBy createdBy ->
            { form | createdBy = createdBy }

        UpdateComment comment ->
            { form | comment = comment }

        DeleteMatcher index ->
            { form | matchers = List.take index form.matchers ++ List.drop (index + 1) form.matchers }

        UpdateMatcherName index name ->
            { form
                | matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | name = name })
                        form.matchers
            }

        UpdateMatcherValue index value ->
            { form
                | matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | value = value })
                        form.matchers
            }

        UpdateMatcherRegex index isRegex ->
            { form
                | matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | isRegex = isRegex })
                        form.matchers
            }


update : SilenceFormMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        CreateSilence silence ->
            ( model
            , Silences.Api.create silence
                |> Cmd.map (SilenceCreate >> MsgForSilenceForm)
            )

        SilenceCreate silence ->
            case silence of
                Success id ->
                    ( model, Navigation.newUrl ("/#/silences/" ++ id) )

                Failure err ->
                    ( model, Navigation.newUrl "/#/silences" )

                Loading ->
                    ( model, Navigation.newUrl "/#/silences" )

        NewSilenceFromMatchers matchers ->
            ( model, Task.perform (NewSilenceFromMatchersAndTime matchers >> MsgForSilenceForm) Time.now )

        NewSilenceFromMatchersAndTime matchers time ->
            ( { model | form = fromMatchersAndTime matchers time }
            , Cmd.none
            )

        FetchSilence silenceId ->
            ( model, Silences.Api.getSilence silenceId (SilenceFetch >> MsgForSilenceForm) )

        SilenceFetch (Success silence) ->
            ( { model | form = fromSilence silence, silence = Ok silence }
            , Task.perform (PreviewSilence >> MsgForSilenceForm) (Task.succeed silence)
            )

        SilenceFetch _ ->
            ( model, Cmd.none )

        PreviewSilence silence ->
            ( { model | silence = Ok { silence | silencedAlerts = Loading } }
            , Alerts.Api.fetchAlerts
                { nullFilter | text = Just (Utils.List.mjoin silence.matchers) }
                |> Cmd.map (AlertGroupsPreview >> MsgForSilenceForm)
            )

        AlertGroupsPreview alertGroups ->
            case model.silence of
                Ok sil ->
                    ( { model | silence = Ok { sil | silencedAlerts = alertGroups } }
                    , Cmd.none
                    )

                Err _ ->
                    ( model, Cmd.none )

        UpdateField fieldMsg ->
            let
                newForm =
                    updateForm fieldMsg model.form

                newSilence =
                    toSilence newForm
            in
                ( { form = newForm, silence = newSilence }, Cmd.none )
