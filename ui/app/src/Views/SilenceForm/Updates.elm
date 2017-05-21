module Views.SilenceForm.Updates exposing (update)

import Alerts.Api
import Silences.Api
import Task
import Time
import Navigation
import Types exposing (Msg(MsgForSilenceForm))
import Utils.Date
import Utils.List
import Utils.Types exposing (ApiResponse(..))
import Utils.Filter exposing (nullFilter)
import Utils.FormValidation exposing (validate, stringNotEmpty)
import Views.SilenceForm.Types
    exposing
        ( Model
        , SilenceForm
        , SilenceFormMsg(..)
        , SilenceFormFieldMsg(..)
        , fromMatchersAndTime
        , fromSilence
        , toSilence
        , emptyMatcher
        )


updateForm : SilenceFormFieldMsg -> SilenceForm -> SilenceForm
updateForm msg form =
    case msg of
        AddMatcher ->
            { form | matchers = (form.matchers ++ [ emptyMatcher ]) }

        UpdateStartsAt time ->
            -- TODO:
            -- Update silence to hold datetime as string, on each pass through
            -- here update an error message "this is invalid", but let them put
            -- it in anyway.
            let
                startsAt =
                    validate Utils.Date.timeFromString time

                durationResult =
                    Result.map2 (-) form.endsAt.validationResult startsAt.validationResult
                        |> Result.mapError (always Utils.FormValidation.Initial)

                durationValue =
                    case durationResult of
                        Ok duration ->
                            Utils.Date.durationFormat duration

                        Err _ ->
                            form.duration.value
            in
                { form
                    | startsAt = startsAt
                    , duration = { value = durationValue, validationResult = durationResult }
                }

        UpdateEndsAt time ->
            let
                endsAt =
                    validate Utils.Date.timeFromString time

                durationResult =
                    Result.map2 (-) endsAt.validationResult form.startsAt.validationResult
                        |> Result.mapError (always Utils.FormValidation.Initial)

                durationValue =
                    case durationResult of
                        Ok duration ->
                            Utils.Date.durationFormat duration

                        Err _ ->
                            form.duration.value
            in
                { form | endsAt = endsAt, duration = { value = durationValue, validationResult = durationResult } }

        UpdateDuration time ->
            let
                duration =
                    validate Utils.Date.parseDuration time

                endsAtResult =
                    Result.map2 (+) form.startsAt.validationResult duration.validationResult
                        |> Result.mapError (always Utils.FormValidation.Initial)

                endsAtValue =
                    case endsAtResult of
                        Ok endsAt ->
                            Utils.Date.timeToString endsAt

                        Err _ ->
                            form.endsAt.value
            in
                { form | endsAt = { value = endsAtValue, validationResult = endsAtResult }, duration = duration }

        UpdateCreatedBy createdBy ->
            { form | createdBy = validate stringNotEmpty createdBy }

        UpdateComment comment ->
            { form | comment = validate stringNotEmpty comment }

        DeleteMatcher index ->
            { form | matchers = List.take index form.matchers ++ List.drop (index + 1) form.matchers }

        UpdateMatcherName index name ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | name = validate stringNotEmpty name })
                        form.matchers
            in
                { form | matchers = matchers }

        UpdateMatcherValue index value ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | value = validate stringNotEmpty value })
                        form.matchers
            in
                { form | matchers = matchers }

        UpdateMatcherRegex index isRegex ->
            let
                matchers =
                    Utils.List.replaceIndex index
                        (\matcher -> { matcher | isRegex = isRegex })
                        form.matchers
            in
                { form | matchers = matchers }


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

                _ ->
                    ( model, Navigation.newUrl "/#/silences" )

        NewSilenceFromMatchers matchers ->
            ( model, Task.perform (NewSilenceFromMatchersAndTime matchers >> MsgForSilenceForm) Time.now )

        NewSilenceFromMatchersAndTime matchers time ->
            let
                form =
                    fromMatchersAndTime matchers time

                silence =
                    toSilence form
            in
                ( Model silence form
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
