module Views.SilenceForm.Views exposing (view)

import Html exposing (Html, a, div, fieldset, label, legend, span, text)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (Silence, SilenceId)
import Utils.Types exposing (ApiData, ApiResponse(..), Matcher)
import Utils.Views exposing (checkbox, error, formField, formInput, iconButtonMsg, textField)
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..))
import Utils.Views exposing (checkbox, formField, formInput, iconButtonMsg, textField)
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceFormMsg(..), SilenceFormFieldMsg(..))


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
            [ div [ class "pa4 black-80" ]
                [ fieldset [ class "ba b--transparent ph0 mh0" ]
                    [ legend [ class "ph0 mh0 fw6" ] [ text title ]
                    , (formField "Start" form.startsAt (UpdateStartsAt >> UpdateField))
                    , div [ class "dib mb2 mr2 w-40" ] [ formField "End" form.endsAt (UpdateEndsAt >> UpdateField) ]
                    , div [ class "dib mb2 mr2 w-40" ] [ formField "Duration" form.duration (UpdateDuration >> UpdateField) ]
                    , div [ class "mt3" ]
                        [ label [ class "f6 b db mb2" ]
                            [ text "Matchers "
                            , span [ class "normal black-60" ] [ text "Alerts affected by this silence. Format: name=value" ]
                            ]
                        , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                        , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                        ]
                    , div [] (List.indexedMap matcherForm form.matchers)
                    , iconButtonMsg "blue" "fa-plus" (AddMatcher |> UpdateField)
                    , formField "Creator" form.createdBy (UpdateCreatedBy >> UpdateField)
                    , textField "Comment" form.comment (UpdateComment >> UpdateField)
                    , div [ class "mt3" ]
                        [ createSilence silence
                        , a
                            [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-red"
                            , onClick resetClick
                            ]
                            [ text "Reset" ]
                        ]
                    ]
                , preview silence
                ]
            ]


createSilence : Result String Silence -> Html SilenceFormMsg
createSilence silenceResult =
    case silenceResult of
        Ok silence ->
            a
                [ class "f6 link br2 ba ph3 pv2 mr2 dib blue"
                , onClick (CreateSilence silence)
                ]
                [ text "Create" ]

        Err msg ->
            span
                [ class "f6 link br2 ba ph3 pv2 mr2 dib red" ]
                [ text "Create" ]


preview : Result String Silence -> Html SilenceFormMsg
preview silenceResult =
    case silenceResult of
        Ok silence ->
            div []
                [ div [ class "mt3" ]
                    [ a
                        [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-green"
                        , onClick (PreviewSilence silence)
                        ]
                        [ text "Show Affected Alerts" ]
                    ]
                , Views.Shared.SilencePreview.view silence
                ]

        Err message ->
            text message


matcherForm : Int -> Matcher -> Html SilenceFormMsg
matcherForm index { name, value, isRegex } =
    div []
        [ formInput name (UpdateMatcherName index)
        , formInput value (UpdateMatcherValue index)
        , checkbox "Regex" isRegex (UpdateMatcherRegex index)
        , iconButtonMsg "dark-red" "fa-trash-o" (DeleteMatcher index)
        ]
        |> Html.map UpdateField
