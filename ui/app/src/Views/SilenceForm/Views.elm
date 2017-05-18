module Views.SilenceForm.Views exposing (view)

import Html exposing (Html, a, div, fieldset, label, legend, span, text, h1, strong, button)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, SilenceId)
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..))
import Utils.FormValidation exposing (ValidatedMatcher)
import Utils.Views exposing (checkbox, error, formInput, iconButtonMsg, textField, validatedFormField, validatedTextField)
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..), SilenceFormFieldMsg(..), SilenceForm)
import Tuple exposing (first)


view : Maybe SilenceId -> Model -> Html SilenceFormMsg
view maybeId { silence, form } =
    let
        ( title, resetClick ) =
            case maybeId of
                Just silenceId ->
                    ( "Edit Silence", FetchSilence silenceId )

                Nothing ->
                    ( "New Silence", NewSilenceFromMatchers [] )
    in
        div []
            [ div []
                [ h1 [] [ text title ]
                , timeInput form.startsAt form.endsAt form.duration
                , matcherInput form.matchers
                , validatedFormField "Creator" form.createdBy inputSectionPadding (UpdateCreatedBy >> UpdateField)
                , validatedTextField "Comment" form.comment inputSectionPadding (UpdateComment >> UpdateField)
                , preview silence
                , silenceActionButtons maybeId silence form resetClick
                ]
            ]


inputSectionPadding : String
inputSectionPadding =
    "mt-5"


timeInput : Result ( String, String ) ( String, x ) -> Result ( String, String ) ( String, x ) -> Result ( String, String ) ( String, x ) -> Html SilenceFormMsg
timeInput startsAt endsAt duration =
    div [ class <| "row " ++ inputSectionPadding ]
        [ (validatedFormField "Start" (Result.map first startsAt) "col-5" (UpdateStartsAt >> UpdateField))
        , (validatedFormField "Duration" (Result.map first duration) "col-2" (UpdateDuration >> UpdateField))
        , (validatedFormField "End" (Result.map first endsAt) "col-5" (UpdateEndsAt >> UpdateField))
        ]


matcherInput : List ValidatedMatcher -> Html SilenceFormMsg
matcherInput matchers =
    div [ class inputSectionPadding ]
        [ div []
            [ label []
                [ strong [] [ text "Matchers " ]
                , span [ class "" ] [ text "Alerts affected by this silence." ]
                ]
            , div [ class "row" ]
                [ label [ class "col-5" ] [ text "Name" ]
                , label [ class "col-5" ] [ text "Value" ]
                ]
            ]
        , div [] (List.indexedMap matcherForm matchers)
        , iconButtonMsg "btn btn-secondary" "fa-plus" (AddMatcher |> UpdateField)
        ]


silenceActionButtons : Maybe String -> Result String Silence -> SilenceForm -> SilenceFormMsg -> Html SilenceFormMsg
silenceActionButtons maybeId silence form resetClick =
    div [ class inputSectionPadding ]
        [ createSilenceBtn maybeId silence form
        , button
            [ class "ml-2 btn btn-danger", onClick resetClick ]
            [ text "Reset" ]
        ]


createSilenceBtn : Maybe String -> Result String Silence -> SilenceForm -> Html SilenceFormMsg
createSilenceBtn maybeId silenceResult form =
    let
        btnTxt =
            case maybeId of
                Just _ ->
                    "Update"

                Nothing ->
                    "Create"
    in
        case (silenceResult) of
            Ok silence ->
                button
                    [ class "btn btn-primary"
                    , onClick (CreateSilence silence)
                    ]
                    [ text btnTxt ]

            _ ->
                span
                    [ class "btn btn-secondary disabled" ]
                    [ text btnTxt ]


preview : Result String Silence -> Html SilenceFormMsg
preview silenceResult =
    div [ class inputSectionPadding ] <|
        case silenceResult of
            Ok silence ->
                [ button [ class "btn btn-outline-success", onClick (PreviewSilence silence) ] [ text "Load affected Alerts" ]
                , Views.Shared.SilencePreview.view silence
                ]

            Err _ ->
                [ div [ class "alert alert-warning" ] [ text "Can not display affected Alerts, Silence is not yet valid." ] ]


matcherForm : Int -> ValidatedMatcher -> Html SilenceFormMsg
matcherForm index { name, value, isRegex } =
    div [ class "row" ]
        [ div [ class "col-5" ] [ validatedFormField "" name "" (UpdateMatcherName index) ]
        , div [ class "col-5" ] [ validatedFormField "" value "" (UpdateMatcherValue index) ]
        , div [ class "col-2 d-flex align-items-center" ]
            [ checkbox "Regex" isRegex (UpdateMatcherRegex index)
            , iconButtonMsg "btn btn-secondary ml-auto" "fa-trash-o" (DeleteMatcher index)
            ]
        ]
        |> Html.map UpdateField
